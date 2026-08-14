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

	"github.com/JohnSBarden/footui/internal/data/models"
)

// CompetitionsRepo persists and queries Competition rows.
type CompetitionsRepo interface {
	UpsertCompetition(ctx context.Context, c models.Competition) error
	ListCompetitions(ctx context.Context) ([]models.Competition, error)
}

// SeasonsRepo persists and queries Season rows.
type SeasonsRepo interface {
	UpsertSeason(ctx context.Context, s models.Season) error
}

// TeamsRepo persists and queries Team rows.
type TeamsRepo interface {
	UpsertTeam(ctx context.Context, t models.Team) error
	FindTeamsByName(ctx context.Context, query string) ([]models.Team, error)
}

// TeamStatsRepo persists and queries TeamSeasonStats rows, and the
// team_competition_season anchor rows that make them queryable per
// competition rather than blended across a team's whole season.
type TeamStatsRepo interface {
	LinkTeamCompetitionSeason(ctx context.Context, teamID, competitionID, seasonID string) error
	UpsertTeamSeasonStats(ctx context.Context, s models.TeamSeasonStats) error
	Standings(ctx context.Context, competitionID, seasonID string) ([]models.TeamSeasonStats, error)
	TeamHistory(ctx context.Context, teamID string) ([]models.TeamSeasonStats, error)
}

// PlayersRepo persists and queries Player rows.
type PlayersRepo interface {
	UpsertPlayer(ctx context.Context, p models.Player) error
}

// PlayerStatsRepo persists and queries PlayerSeasonStats rows.
type PlayerStatsRepo interface {
	UpsertPlayerSeasonStats(ctx context.Context, s models.PlayerSeasonStats) error
	RosterForTeamSeason(ctx context.Context, teamID, competitionID, seasonID string) ([]models.PlayerSeasonStats, error)
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
}
