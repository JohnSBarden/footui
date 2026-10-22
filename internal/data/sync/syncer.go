// Package sync orchestrates a refresh: fetch from a provider, then upsert
// through storage.Repositories. It depends only on the two contracts.
package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
)

// Source is the slice of the provider contract a league-season sync needs.
type Source interface {
	provider.StandingsProvider
	provider.TeamsProvider
	provider.LeaguePlayersProvider
}

// DefaultMaxConsecutiveFailures is how many units may fail back-to-back
// before a run gives up on the rest, so an unreachable host doesn't cost a
// timeout per unit. Consecutive, not total: scattered failures are fine.
const DefaultMaxConsecutiveFailures = 3

// Syncer runs refreshes against a Source and a storage.Repositories.
type Syncer struct {
	source Source
	repos  storage.Repositories
	// providerName is recorded in the sync log.
	providerName string
	// MaxConsecutiveFailures bounds how many units may fail in a row
	// before the run gives up. Zero means DefaultMaxConsecutiveFailures;
	// negative disables the check entirely.
	MaxConsecutiveFailures int
	// MaxAge skips a competition-season whose last successful sync is
	// newer than this. Zero refreshes everything.
	MaxAge time.Duration
	// KeepFinished leaves alone a past season that was synced after it
	// ended, whatever MaxAge says.
	KeepFinished bool
	// Now is the clock, injectable for tests.
	Now func() time.Time
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func New(source Source, repos storage.Repositories) *Syncer {
	return &Syncer{source: source, repos: repos, providerName: "unknown"}
}

// WithProviderName sets the provider name recorded in the sync log.
func (s *Syncer) WithProviderName(name string) *Syncer {
	s.providerName = name
	return s
}

func (s *Syncer) maxConsecutiveFailures() int {
	if s.MaxConsecutiveFailures == 0 {
		return DefaultMaxConsecutiveFailures
	}
	return s.MaxConsecutiveFailures
}

// Result summarizes what a SyncCompetitionSeason call did, for CLI output.
type Result struct {
	Teams       int
	TeamStats   int
	Players     int
	PlayerStats int
	// Matches counts the matches a competition-year fetch stored.
	Matches int
}

// SyncCompetitionSeason fetches and stores one competition-season.
func (s *Syncer) SyncCompetitionSeason(ctx context.Context, comp models.Competition, season models.Season) (Result, error) {
	startedAt := time.Now().UTC()
	res, err := s.syncCompetitionSeason(ctx, comp, season)
	s.recordRun(ctx, comp, season, startedAt, res, err)
	return res, err
}

// recordRun writes the attempt, failures included, to the sync log. A
// logging error is deliberately swallowed: it mustn't fail a good sync.
func (s *Syncer) recordRun(ctx context.Context, comp models.Competition, season models.Season,
	startedAt time.Time, res Result, syncErr error) {

	run := models.SyncRun{
		Provider:      s.providerName,
		CompetitionID: comp.ID,
		SeasonID:      season.ID,
		StartedAt:     startedAt,
		FinishedAt:    time.Now().UTC(),
		Status:        models.SyncStatusOK,
		RowsUpserted:  res.TeamStats + res.PlayerStats,
	}
	if syncErr != nil {
		run.Status = failureStatus(syncErr)
		run.Error = syncErr.Error()
	}

	// A cancelled context can't write anything, so don't try.
	if ctx.Err() != nil {
		return
	}
	_ = s.repos.RecordSyncRun(ctx, run)
}

func (s *Syncer) syncCompetitionSeason(ctx context.Context, comp models.Competition, season models.Season) (Result, error) {
	var res Result

	compRef := provider.CompetitionRef{CompetitionID: comp.ID, Name: comp.Name}
	seasonRef := provider.SeasonRef{SeasonID: season.ID, Label: season.Label}

	// Fetch everything before writing anything, so a failed refresh leaves
	// no empty season rows that would then show as cached. The source
	// caches per season, so these calls are one round trip.
	teams, err := s.source.Teams(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch teams: %w", err)
	}

	standings, err := s.source.Standings(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch standings: %w", err)
	}

	players, err := s.source.Players(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch players: %w", err)
	}

	playerStats, err := s.source.AllRosterStats(ctx, compRef, seasonRef)
	if err != nil {
		return res, fmt.Errorf("sync: fetch roster stats: %w", err)
	}

	var matches []models.Match
	if ms, ok := s.source.(provider.SeasonMatchesProvider); ok {
		if matches, err = ms.SeasonMatches(ctx, compRef, seasonRef); err != nil {
			return res, fmt.Errorf("sync: fetch matches: %w", err)
		}
	}

	// Native provider IDs are mapped to canonical ones; see identity.go.
	var ids *resolver
	if hasNativeIDs(teams) {
		if ids, err = newResolver(ctx, s.repos, comp); err != nil {
			return res, fmt.Errorf("sync: resolve identities: %w", err)
		}
		if teams, err = ids.teams(ctx, teams); err != nil {
			return res, fmt.Errorf("sync: resolve teams: %w", err)
		}
		for i := range standings {
			standings[i].TeamID = ids.canonicalTeam(standings[i].TeamID)
		}
		var playerIDs map[string]string
		if players, playerIDs, err = ids.players(ctx, players, playerStats); err != nil {
			return res, fmt.Errorf("sync: resolve players: %w", err)
		}
		for i := range playerStats {
			playerStats[i].TeamID = ids.canonicalTeam(playerStats[i].TeamID)
			if c, ok := playerIDs[playerStats[i].PlayerID]; ok {
				playerStats[i].PlayerID = c
			}
		}
		for i := range matches {
			matches[i].HomeTeamID = ids.canonicalTeam(matches[i].HomeTeamID)
			matches[i].AwayTeamID = ids.canonicalTeam(matches[i].AwayTeamID)
		}
	}
	for i := range matches {
		if matches[i].ID, err = canonicalMatchID(ctx, s.repos, matches[i].ID); err != nil {
			return res, fmt.Errorf("sync: resolve match: %w", err)
		}
	}

	if err := s.repos.UpsertCompetition(ctx, comp); err != nil {
		return res, err
	}
	if err := s.repos.UpsertSeason(ctx, season); err != nil {
		return res, err
	}
	// The fetch replaces the cached season. One with no clubs means the
	// source has nothing, never that the season was emptied.
	if len(teams) > 0 {
		if err := s.repos.ClearCompetitionSeason(ctx, comp.ID, season.ID); err != nil {
			return res, fmt.Errorf("sync: clear season: %w", err)
		}
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

	for _, st := range standings {
		if err := s.repos.UpsertTeamSeasonStats(ctx, st); err != nil {
			return res, fmt.Errorf("sync: upsert team_season_stats %s: %w", st.TeamID, err)
		}
		res.TeamStats++
	}

	for _, p := range players {
		if err := s.repos.UpsertPlayer(ctx, p); err != nil {
			return res, fmt.Errorf("sync: upsert player %s: %w", p.ID, err)
		}
		res.Players++
	}

	for _, ps := range playerStats {
		if err := s.repos.UpsertPlayerSeasonStats(ctx, ps); err != nil {
			return res, fmt.Errorf("sync: upsert player_season_stats %s: %w", ps.PlayerID, err)
		}
		res.PlayerStats++
	}

	if err := s.repos.UpsertMatches(ctx, matches); err != nil {
		return res, fmt.Errorf("sync: upsert matches: %w", err)
	}
	res.Matches = len(matches)

	s.applyProviderColors(ctx, compRef, seasonRef, ids)
	return res, nil
}

// colorSource is a provider that knows clubs' brand colours.
type colorSource interface {
	TeamColors(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamColors, error)
}

// applyProviderColors stores a provider's brand colours for clubs that
// have none yet; curated ones win. Failures don't fail the refresh.
func (s *Syncer) applyProviderColors(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef, ids *resolver) {
	cs, ok := s.source.(colorSource)
	if !ok {
		return
	}
	colors, err := cs.TeamColors(ctx, comp, season)
	if err != nil {
		return
	}
	for _, c := range colors {
		if ids != nil {
			mapped, known := ids.teamOf[c.TeamID]
			if !known {
				continue // a club not in this season's table
			}
			c.TeamID = mapped
		}
		if _, err := s.repos.GetTeamColors(ctx, c.TeamID); errors.Is(err, storage.ErrNotFound) {
			_ = s.repos.UpsertTeamColors(ctx, c)
		}
	}
}

// errSkippedRateLimited marks the units a run never attempted because the
// provider had rate-limited it.
var errSkippedRateLimited = fmt.Errorf("skipped: the provider is rate-limiting footui: %w", provider.ErrRateLimited)

// failureStatus logs rate limiting distinctly: it suspends automatic
// refreshes for a while.
func failureStatus(err error) string {
	if errors.Is(err, provider.ErrRateLimited) {
		return models.SyncStatusRateLimited
	}
	return models.SyncStatusFailed
}

func hasNativeIDs(teams []models.Team) bool {
	for _, t := range teams {
		if _, _, ok := nativeID(t.ID); ok {
			return true
		}
	}
	return false
}

// UnitResult is the outcome of syncing one competition-season.
type UnitResult struct {
	Competition models.Competition
	Season      models.Season
	// Year is set for a competition-year of international matches, whose
	// Season is then only a label ("2025") for display.
	Year   int
	Result Result
	Err    error
	// Skipped marks a unit never attempted because the run gave up.
	Skipped bool
	// Fresh marks a unit left alone because its cache is new enough; a
	// good outcome, not a failure.
	Fresh bool
	// CachedAt is when a Fresh unit was last successfully synced.
	CachedAt time.Time
}

// Failed reports whether this unit errored.
func (u UnitResult) Failed() bool { return u.Err != nil }

// Progress is called with each unit's outcome as it completes; nil is
// allowed.
type Progress func(UnitResult)

// SyncMany syncs every combination of the given competitions and season
// start years. One unit failing does not abort the others; only a
// cancelled context stops the run.
func (s *Syncer) SyncMany(ctx context.Context, comps []models.Competition, seasonYears []string, progress Progress) []UnitResult {
	jobs := make([]Job, 0, len(comps))
	for _, c := range comps {
		jobs = append(jobs, Job{Competition: c, Seasons: seasonYears})
	}
	return s.SyncJobs(ctx, jobs, progress)
}

// Job is one competition and the season start years of it to sync.
type Job struct {
	Competition models.Competition
	Seasons     []string
}

// Units counts the competition-seasons in a set of jobs.
func Units(jobs []Job) int {
	n := 0
	for _, j := range jobs {
		n += len(j.Seasons)
	}
	return n
}

// SyncJobs is SyncMany for jobs whose season lists differ.
func (s *Syncer) SyncJobs(ctx context.Context, jobs []Job, progress Progress) []UnitResult {
	results := make([]UnitResult, 0, Units(jobs))

	record := func(u UnitResult) {
		results = append(results, u)
		if progress != nil {
			progress(u)
		}
	}

	consecutiveFailures := 0
	limit := s.maxConsecutiveFailures()
	gaveUp := false
	stopReason := fmt.Errorf("skipped after %d consecutive failures", limit)

	for _, job := range jobs {
		comp := job.Competition
		for _, year := range job.Seasons {
			season := NewSeason(comp.ID, year)
			season.CalendarYear = comp.CalendarYear

			if err := ctx.Err(); err != nil {
				record(UnitResult{Competition: comp, Season: season, Err: err})
				return results
			}

			if gaveUp {
				record(UnitResult{Competition: comp, Season: season, Skipped: true, Err: stopReason})
				continue
			}

			if cachedAt, fresh := s.isFresh(ctx, comp, season); fresh {
				record(UnitResult{Competition: comp, Season: season,
					Fresh: true, CachedAt: cachedAt})
				// Neither breaks nor extends a failure streak.
				continue
			}

			res, err := s.SyncCompetitionSeason(ctx, comp, season)
			record(UnitResult{Competition: comp, Season: season, Result: res, Err: err})

			if err != nil {
				consecutiveFailures++
				switch {
				case errors.Is(err, provider.ErrRateLimited):
					// The source asked footui to stop; every further
					// request would only prolong that.
					gaveUp, stopReason = true, errSkippedRateLimited
				case limit > 0 && consecutiveFailures >= limit:
					gaveUp = true
				}
			} else {
				consecutiveFailures = 0
			}
		}
	}

	return results
}

// isFresh reports whether a competition-season was successfully synced
// recently enough to leave alone. Only a successful run counts.
func (s *Syncer) isFresh(ctx context.Context, comp models.Competition, season models.Season) (time.Time, bool) {
	if s.MaxAge <= 0 && !s.KeepFinished {
		return time.Time{}, false
	}

	run, err := s.repos.LatestSyncRun(ctx, comp.ID, season.ID)
	if err != nil || !run.Succeeded() {
		return time.Time{}, false
	}
	if s.KeepFinished && finishedBy(comp.ID, season.ID, run.FinishedAt) {
		return run.FinishedAt, true
	}
	if s.MaxAge <= 0 || s.now().Sub(run.FinishedAt) > s.MaxAge {
		return time.Time{}, false
	}
	return run.FinishedAt, true
}

// Summary aggregates a SyncMany run for reporting.
type Summary struct {
	Units     int
	Succeeded int
	Failed    int
	Skipped   int
	// Fresh counts units left alone because their data was recent enough.
	Fresh  int
	Totals Result
}

// Summarize totals up a run's results.
func Summarize(results []UnitResult) Summary {
	var s Summary
	s.Units = len(results)
	for _, r := range results {
		if r.Fresh {
			s.Fresh++
			continue
		}
		if r.Skipped {
			s.Skipped++
			continue
		}
		if r.Failed() {
			s.Failed++
			continue
		}
		s.Succeeded++
		s.Totals.Teams += r.Result.Teams
		s.Totals.TeamStats += r.Result.TeamStats
		s.Totals.Players += r.Result.Players
		s.Totals.PlayerStats += r.Result.PlayerStats
		s.Totals.Matches += r.Result.Matches
	}
	return s
}
