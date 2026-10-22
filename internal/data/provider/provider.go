// Package provider defines the source contract. Implementations map their
// source's shapes into models types; no provider-specific type leaks out.
// Interfaces are small so a source can implement just the slice it has.
package provider

import (
	"context"
	"errors"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// CompetitionRef identifies a competition to a provider.
type CompetitionRef struct {
	CompetitionID string
	Name          string
}

// SeasonRef identifies a season within a competition.
type SeasonRef struct {
	SeasonID string
	Label    string
}

// StandingsProvider supplies a league's standings-table team stats.
type StandingsProvider interface {
	Standings(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.TeamSeasonStats, error)
}

// TeamsProvider supplies the teams (identity, not stats) in a
// competition-season.
type TeamsProvider interface {
	Teams(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Team, error)
}

// LeaguePlayersProvider supplies an entire competition-season's player
// identities and stat lines in one call, rather than per team.
type LeaguePlayersProvider interface {
	Players(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Player, error)
	AllRosterStats(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.PlayerSeasonStats, error)
}

// NationalTeamsProvider supplies the national teams of one gender ("men"
// or "women").
type NationalTeamsProvider interface {
	NationalTeams(ctx context.Context, gender string) ([]models.Team, error)
}

// MatchesProvider supplies every match of a competition in one calendar
// year, and the teams that played them. A match's SeasonID is its edition,
// which can differ from year (Euro 2020 was played in 2021).
type MatchesProvider interface {
	CompetitionMatches(ctx context.Context, comp CompetitionRef, year int) ([]models.Match, []models.Team, error)
}

// SeasonMatchesProvider supplies every match of one competition-season.
// Team IDs are the provider's, as in Teams.
type SeasonMatchesProvider interface {
	SeasonMatches(ctx context.Context, comp CompetitionRef, season SeasonRef) ([]models.Match, error)
}

// SquadMember is one player on a provider's club roster.
type SquadMember struct {
	ID       string
	Name     string
	Position string // "G", "D", "M", "F", or "" when the provider gave none
}

// CareerSeason is one competition-season a provider has a player's stats
// for.
type CareerSeason struct {
	CompetitionID string // canonical
	Year          int    // the season's start year
}

// PlayerSearcher finds players at the source and lists their careers.
type PlayerSearcher interface {
	SearchPlayers(ctx context.Context, query string) ([]models.FoundPlayer, error)
	// PlayerCareer lists, newest first, the seasons of the player's
	// career in competitions this source knows; others are left out.
	PlayerCareer(ctx context.Context, id string) ([]models.CareerSeason, error)
}

// ErrRateLimited means the source kept refusing after the retries. A
// refresh that gets this stops rather than carrying on.
var ErrRateLimited = errors.New("rate limited")

// ErrNoTable means the source has no table for that season: before the
// competition existed, or an edition that was knockout-only.
var ErrNoTable = errors.New("no table for that season")

// ErrNotFound means the source has no such thing; asking again won't
// change that.
var ErrNotFound = errors.New("not found")

// ErrIncomplete means the source truncated the data. What is returned
// alongside it is worth storing but must not be recorded as complete.
var ErrIncomplete = errors.New("incomplete")

// Timeout is a request that got no response in time. Each source's
// timeout error implements it, for errors.As.
type Timeout interface {
	error
	TimedOutAfter() time.Duration
}

// Exhausted is a fetch that failed on every attempt it was allowed.
type Exhausted interface {
	error
	AttemptsMade() int
}
