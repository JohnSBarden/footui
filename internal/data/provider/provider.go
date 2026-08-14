// Package provider defines the source contract: the boundary between an
// external data source (a scraper, a future paid API) and everything else
// in footui. A provider implementation maps its source's own shapes into
// internal/data/models types and nothing else in the codebase — the sync
// layer, storage layer, and TUI — ever sees a provider-specific type.
//
// Interfaces are small and domain-scoped rather than one monolithic
// Provider, so a source that's only good at one thing (e.g. an xG-only
// source) can implement just that slice of the contract. A single
// implementation is free to satisfy several interfaces at once, as
// internal/data/provider/understat does.
package provider

import (
	"context"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// CompetitionRef identifies a competition to a provider using whatever
// reference that provider needs (a canonical ID at minimum).
type CompetitionRef struct {
	CompetitionID string
	Name          string
}

// SeasonRef identifies a season within a competition.
type SeasonRef struct {
	SeasonID string
	Label    string
}

// TeamRef identifies a team.
type TeamRef struct {
	TeamID string
	Name   string
}

// PlayerRef identifies a player.
type PlayerRef struct {
	PlayerID string
	Name     string
}

// StandingsProvider supplies league-standings-shaped team stats: results,
// points, goal difference — the fields needed to render a standings table.
type StandingsProvider interface {
	Standings(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.TeamSeasonStats, error)
}

// TeamStatsProvider supplies a single team's fuller season stat line
// (possession, fouls, cards, shots, etc.) beyond bare standings.
type TeamStatsProvider interface {
	TeamSeasonStats(ctx context.Context, comp CompetitionRef, season SeasonRef, team TeamRef) (models.TeamSeasonStats, error)
}

// XGProvider supplies expected-goals figures, which most standings sources
// don't carry. A sync pass merges XGProvider output into existing
// TeamSeasonStats/PlayerSeasonStats rows rather than overwriting them.
type XGProvider interface {
	TeamXG(ctx context.Context, comp CompetitionRef, season SeasonRef, team TeamRef) (xg, xga float64, err error)
	PlayerXG(ctx context.Context, comp CompetitionRef, season SeasonRef, player PlayerRef) (xg, xa float64, err error)
}

// PlayerStatsProvider supplies a competition+season roster with per-player
// season aggregates.
type PlayerStatsProvider interface {
	RosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef, team TeamRef) ([]models.PlayerSeasonStats, error)
}

// TrophyProvider supplies a team's honours history.
type TrophyProvider interface {
	TeamTrophies(ctx context.Context, team TeamRef) ([]models.Trophy, error)
}

// TeamsProvider supplies the Team identity rows (name, country, gender —
// not stats) participating in a competition+season.
type TeamsProvider interface {
	Teams(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Team, error)
}

// LeaguePlayersProvider is a bulk-fetch counterpart to PlayerStatsProvider,
// for sources (like understat.com) that return an entire league/season's
// player identities and stat lines in a single call rather than per team.
type LeaguePlayersProvider interface {
	Players(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Player, error)
	AllRosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.PlayerSeasonStats, error)
}

// TeamResolver resolves a free-text team name to candidate teams, ordered
// by match confidence (best match first).
type TeamResolver interface {
	ResolveTeam(ctx context.Context, name string) ([]TeamRef, error)
}
