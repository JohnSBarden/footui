// Package storage defines the backend contract: the boundary between the
// sync layer (which writes canonical models fetched from a provider) and
// everything that reads persisted data (the CLI and TUI). Both sides depend
// only on these interfaces — never on internal/storage/sqlite's concrete
// types — so the on-disk representation can change without touching the
// sync layer or the TUI, and both can be tested against a fake
// implementation with no real database involved.
package storage

import (
	"context"
	"errors"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// ErrNotFound is returned by the single-row Get* queries when nothing is
// cached for the requested ID. Callers distinguish "no data cached yet,
// tell the user to run footy refresh" from a real backend failure with
// errors.Is(err, storage.ErrNotFound).
var ErrNotFound = errors.New("storage: not found")

// CompetitionsRepo persists and queries Competition rows.
type CompetitionsRepo interface {
	UpsertCompetition(ctx context.Context, c models.Competition) error
	ListCompetitions(ctx context.Context) ([]models.Competition, error)
	// GetCompetition returns the competition with the given ID, or
	// ErrNotFound if no such row is cached.
	GetCompetition(ctx context.Context, id string) (models.Competition, error)
	// FindCompetitionsByName returns competitions whose name contains
	// query (case-insensitive), for resolving `footy league <name>`.
	FindCompetitionsByName(ctx context.Context, query string) ([]models.Competition, error)
	// GetCompetitions resolves many competition IDs at once, keyed by ID.
	GetCompetitions(ctx context.Context, ids []string) (map[string]models.Competition, error)
}

// SeasonsRepo persists and queries Season rows.
type SeasonsRepo interface {
	UpsertSeason(ctx context.Context, s models.Season) error
	// ListSeasons returns a competition's cached seasons, most recent
	// first, so a caller can take the N most recent without knowing how
	// season labels are formatted.
	ListSeasons(ctx context.Context, competitionID string) ([]models.Season, error)
	// GetSeasons resolves many season IDs at once, keyed by ID. A team's
	// history spans several competitions' seasons, so it can't be served
	// by ListSeasons.
	GetSeasons(ctx context.Context, ids []string) (map[string]models.Season, error)
}

// TeamsRepo persists and queries Team rows.
type TeamsRepo interface {
	UpsertTeam(ctx context.Context, t models.Team) error
	FindTeamsByName(ctx context.Context, query string) ([]models.Team, error)
	// GetTeam returns one team by ID, or ErrNotFound.
	GetTeam(ctx context.Context, id string) (models.Team, error)
	// GetTeams resolves many team IDs at once, keyed by ID. Stats rows
	// carry only team IDs, so a screen rendering a standings table needs
	// this to label its rows without one query per row.
	GetTeams(ctx context.Context, ids []string) (map[string]models.Team, error)
	// ListTeams returns every cached team, ordered by ID. Colour seeding
	// is driven by what's cached rather than by the seed file, so this is
	// how it enumerates its work.
	ListTeams(ctx context.Context) ([]models.Team, error)
}

// SyncLogRepo records refresh attempts and answers staleness questions.
type SyncLogRepo interface {
	RecordSyncRun(ctx context.Context, r models.SyncRun) error
	// LatestSyncRun returns the most recent attempt for one
	// competition-season, or ErrNotFound if it has never been attempted.
	LatestSyncRun(ctx context.Context, competitionID, seasonID string) (models.SyncRun, error)
}

// TrophiesRepo persists and queries a club's honours.
type TrophiesRepo interface {
	InsertTrophy(ctx context.Context, t models.Trophy) error
	// DeleteTrophiesBySource removes a team's trophies from one source, so
	// re-seeding corrected data replaces the old rows instead of
	// duplicating them.
	DeleteTrophiesBySource(ctx context.Context, teamID, source string) error
	// TeamTrophies returns a team's honours, most recent season first.
	TeamTrophies(ctx context.Context, teamID string) ([]models.Trophy, error)
}

// TeamColorsRepo persists and queries a club's brand colors.
type TeamColorsRepo interface {
	UpsertTeamColors(ctx context.Context, c models.TeamColors) error
	// GetTeamColors returns a team's colors, or ErrNotFound when none are
	// seeded — a normal, expected state for a club not yet curated.
	GetTeamColors(ctx context.Context, teamID string) (models.TeamColors, error)
}

// TeamStatsRepo persists and queries TeamSeasonStats rows, and the
// team_competition_season anchor rows that make them queryable per
// competition rather than blended across a team's whole season.
type TeamStatsRepo interface {
	LinkTeamCompetitionSeason(ctx context.Context, teamID, competitionID, seasonID string) error
	UpsertTeamSeasonStats(ctx context.Context, s models.TeamSeasonStats) error
	Standings(ctx context.Context, competitionID, seasonID string) ([]models.TeamSeasonStats, error)
	// TeamHistory returns every cached season for a team, across every
	// competition, most recent first. Rows stay one-per-(competition,
	// season): callers group them, they are never pre-blended.
	TeamHistory(ctx context.Context, teamID string) ([]models.TeamSeasonStats, error)
	// TeamSeasonStatExtras returns the long-tail key/value stats for one
	// team-competition-season, or an empty map when none were supplied.
	// Extras are kept out of Standings and TeamHistory because a standings
	// table needs 20 rows of core columns, not 20 rows plus seven
	// key/value lookups each.
	TeamSeasonStatExtras(ctx context.Context, teamID, competitionID, seasonID string) (map[string]float64, error)
}

// PlayersRepo persists and queries Player rows.
type PlayersRepo interface {
	UpsertPlayer(ctx context.Context, p models.Player) error
	// GetPlayer returns one player by ID, or ErrNotFound.
	GetPlayer(ctx context.Context, id string) (models.Player, error)
	// GetPlayers resolves many player IDs at once, keyed by ID, so a
	// roster table labels its rows in one query.
	GetPlayers(ctx context.Context, ids []string) (map[string]models.Player, error)
	// FindPlayersByName backs free-text player search.
	FindPlayersByName(ctx context.Context, query string) ([]models.Player, error)
}

// PlayerStatsRepo persists and queries PlayerSeasonStats rows.
type PlayerStatsRepo interface {
	UpsertPlayerSeasonStats(ctx context.Context, s models.PlayerSeasonStats) error
	RosterForTeamSeason(ctx context.Context, teamID, competitionID, seasonID string) ([]models.PlayerSeasonStats, error)
	// PlayerHistory returns a player's season lines across every team and
	// competition, most recent first.
	PlayerHistory(ctx context.Context, playerID string) ([]models.PlayerSeasonStats, error)
}

// Repositories bundles every repo interface — the shape the sync layer and
// CLI/TUI construction code depend on, so wiring up storage is passing one
// value rather than six.
type Repositories interface {
	CompetitionsRepo
	SeasonsRepo
	TeamsRepo
	TeamStatsRepo
	PlayersRepo
	PlayerStatsRepo
	TeamColorsRepo
	TrophiesRepo
	SyncLogRepo
}
