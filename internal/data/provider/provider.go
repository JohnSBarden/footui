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
	"errors"
	"time"

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

// StandingsProvider supplies league-standings-shaped team stats: results,
// points, goal difference — the fields needed to render a standings table.
type StandingsProvider interface {
	Standings(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.TeamSeasonStats, error)
}

// TeamsProvider supplies the Team identity rows (name, country, gender —
// not stats) participating in a competition+season.
type TeamsProvider interface {
	Teams(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Team, error)
}

// LeaguePlayersProvider supplies an entire competition-season's player
// identities and stat lines in one call, rather than per team.
type LeaguePlayersProvider interface {
	Players(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Player, error)
	AllRosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.PlayerSeasonStats, error)
}

// NationalTeamsProvider supplies the directory of national teams of one
// gender ("men" or "women"), so they can be searched before any of their
// matches are fetched.
type NationalTeamsProvider interface {
	NationalTeams(ctx context.Context, gender string) ([]models.Team, error)
}

// MatchesProvider supplies every match of a competition in one calendar
// year — every nation's, in one call — with per-team stats where the
// source has them, and the teams that played them. Each match's SeasonID
// is the edition it belongs to, which can differ from year (Euro 2020 was
// played in 2021).
type MatchesProvider interface {
	CompetitionMatches(ctx context.Context, comp CompetitionRef, year int) ([]models.Match, []models.Team, error)
}

// SeasonMatchesProvider supplies every match of one competition-season,
// for a source whose season payload already carries them (understat's
// league data). Team IDs are the provider's, as in Teams.
type SeasonMatchesProvider interface {
	SeasonMatches(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Match, error)
}

// SquadMember is one player on a provider's club roster: enough to match
// them to a cached player and learn the provider's ID and position.
type SquadMember struct {
	ID       string
	Name     string
	Position string // "G", "D", "M", "F", or "" when the provider gave none
}

// CareerSeason is one competition-season a provider has a player's stats
// for: what a player's whole history is fetched from.
type CareerSeason struct {
	CompetitionID string // canonical
	Year          int    // the season's start year
}

// PlayerSearcher finds players by name at the source, beyond what is
// cached, and lists the competition-seasons a player appeared in, so the
// ones footui covers can be fetched.
type PlayerSearcher interface {
	SearchPlayers(ctx context.Context, query string) ([]models.FoundPlayer, error)
	// PlayerCareer lists, newest first, the seasons of the player's
	// career in competitions this source knows; others are left out.
	PlayerCareer(ctx context.Context, id string) ([]models.CareerSeason, error)
}

// ErrRateLimited means the source refused footui for asking too often, and
// kept refusing after the polite retries. Nothing more should be asked of
// it for a while: a refresh that gets this stops rather than carrying on.
var ErrRateLimited = errors.New("rate limited")

// ErrNoTable means the source has no table for that season: before the
// competition existed, or an edition that was knockout-only.
var ErrNoTable = errors.New("no table for that season")

// ErrNotFound means the source has no such thing: a competition, season
// or team it doesn't hold, or a player-season it kept no stats for.
// Asking again won't change that.
var ErrNotFound = errors.New("not found")

// ErrIncomplete means a fetch returned data that is known to be partial
// (the source truncated it). The data returned alongside it is still
// worth storing, but must not be recorded as complete.
var ErrIncomplete = errors.New("incomplete")

// Timeout is a request that got no response within After. Each source's
// own timeout error implements it, so a caller can phrase the failure
// without knowing which source it came from (errors.As with a Timeout).
type Timeout interface {
	error
	TimedOutAfter() time.Duration
}

// Exhausted is a fetch that failed on every attempt it was allowed. Each
// source's own give-up error implements it.
type Exhausted interface {
	error
	AttemptsMade() int
}
