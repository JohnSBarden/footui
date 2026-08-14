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
