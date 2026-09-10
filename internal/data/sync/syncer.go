// Package sync orchestrates a refresh: fetch from a provider, then upsert
// through the storage.Repositories backend contract. It depends on both
// contract boundaries (internal/data/provider, internal/storage) and on
// neither's concrete implementation — it can be tested with fakes for both.
package sync

import (
	"context"
	"fmt"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
)

// Source is the slice of the provider contract a full league/season sync
// needs. Any provider implementing all four interfaces can be synced with
// Syncer without Syncer knowing which concrete provider it is.
type Source interface {
	provider.StandingsProvider
	provider.TeamsProvider
	provider.LeaguePlayersProvider
}

// Syncer runs refreshes against a Source and a storage.Repositories.
type Syncer struct {
	source Source
	repos  storage.Repositories
}

func New(source Source, repos storage.Repositories) *Syncer {
	return &Syncer{source: source, repos: repos}
}

// Result summarizes what a SyncCompetitionSeason call did, for CLI output
// and eventually the TUI's staleness indicator (sync_log in the schema).
type Result struct {
	Teams       int
	TeamStats   int
	Players     int
	PlayerStats int
}

// SyncCompetitionSeason fetches and persists everything Source can supply
// for one competition+season: teams, standings (including xG where the
// source has it), and player rosters with season stats.
func (s *Syncer) SyncCompetitionSeason(ctx context.Context, comp models.Competition, season models.Season) (Result, error) {
	var res Result

	if err := s.repos.UpsertCompetition(ctx, comp); err != nil {
		return res, err
	}
	if err := s.repos.UpsertSeason(ctx, season); err != nil {
		return res, err
	}

	compRef := provider.CompetitionRef{CompetitionID: comp.ID, Name: comp.Name}
	seasonRef := provider.SeasonRef{SeasonID: season.ID, Label: season.Label}

	teams, err := s.source.Teams(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch teams: %w", err)
	}
	for _, t := range teams {
		if err := s.repos.UpsertTeam(ctx, t); err != nil {
			return res, fmt.Errorf("sync: upsert team %s: %w", t.ID, err)
		}
		if err := s.repos.LinkTeamCompetitionSeason(ctx, t.ID, comp.ID, season.ID); err != nil {
			return res, fmt.Errorf("sync: link team %s: %w", t.ID, err)
		}
		res.Teams++
	}

	standings, err := s.source.Standings(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch standings: %w", err)
	}
	for _, st := range standings {
		if err := s.repos.UpsertTeamSeasonStats(ctx, st); err != nil {
			return res, fmt.Errorf("sync: upsert team_season_stats %s: %w", st.TeamID, err)
		}
		res.TeamStats++
	}

	players, err := s.source.Players(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch players: %w", err)
	}
	for _, p := range players {
		if err := s.repos.UpsertPlayer(ctx, p); err != nil {
			return res, fmt.Errorf("sync: upsert player %s: %w", p.ID, err)
		}
		res.Players++
	}

	playerStats, err := s.source.AllRosterStats(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch roster stats: %w", err)
	}
	for _, ps := range playerStats {
		if err := s.repos.UpsertPlayerSeasonStats(ctx, ps); err != nil {
			return res, fmt.Errorf("sync: upsert player_season_stats %s: %w", ps.PlayerID, err)
		}
		res.PlayerStats++
	}

	return res, nil
}

// UnitResult is the outcome of syncing one competition-season. Err being
// non-nil does not stop the rest of a run — it is recorded here and
// reported at the end.
type UnitResult struct {
	Competition models.Competition
	Season      models.Season
	Result      Result
	Err         error
}

// Failed reports whether this unit errored.
func (u UnitResult) Failed() bool { return u.Err != nil }

// Progress is called with each unit's outcome as soon as it completes.
// A full refresh spends most of its time waiting on the network, so
// reporting only at the end leaves the user staring at a frozen terminal
// for minutes. nil is allowed and means "no reporting".
type Progress func(UnitResult)

// SyncMany syncs every combination of the given competitions and season
// start years, isolating failures: one competition (or one season of one
// competition) failing does not abort the others. progress, if non-nil,
// is called as each unit finishes.
//
// That isolation is the whole point of the method. A full refresh spans
// dozens of upstream requests against an undocumented endpoint, and the
// expected failure mode is partial — one league's season missing, a
// transient network error — not total. Aborting the run would throw away
// every competition already fetched and leave the cache half-populated
// with no record of why.
//
// A cancelled context is the one thing that does stop the run: that is the
// user pressing ctrl+c, not an upstream problem.
func (s *Syncer) SyncMany(ctx context.Context, comps []models.Competition, seasonYears []string, progress Progress) []UnitResult {
	results := make([]UnitResult, 0, len(comps)*len(seasonYears))

	record := func(u UnitResult) {
		results = append(results, u)
		if progress != nil {
			progress(u)
		}
	}

	for _, comp := range comps {
		for _, year := range seasonYears {
			season := NewSeason(comp.ID, year)

			if err := ctx.Err(); err != nil {
				record(UnitResult{Competition: comp, Season: season, Err: err})
				return results
			}

			res, err := s.SyncCompetitionSeason(ctx, comp, season)
			record(UnitResult{Competition: comp, Season: season, Result: res, Err: err})
		}
	}

	return results
}

// Summary aggregates a SyncMany run for reporting.
type Summary struct {
	Units     int
	Succeeded int
	Failed    int
	Totals    Result
}

// Summarize totals up a run's results.
func Summarize(results []UnitResult) Summary {
	var s Summary
	s.Units = len(results)
	for _, r := range results {
		if r.Failed() {
			s.Failed++
			continue
		}
		s.Succeeded++
		s.Totals.Teams += r.Result.Teams
		s.Totals.TeamStats += r.Result.TeamStats
		s.Totals.Players += r.Result.Players
		s.Totals.PlayerStats += r.Result.PlayerStats
	}
	return s
}
