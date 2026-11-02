// Package storage defines the backend contract between the sync layer and
// readers (CLI, TUI); neither depends on the sqlite package's concrete types.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// ErrNotFound is returned by single-row Get* queries when nothing is cached
// for the ID; callers check it with errors.Is to prompt a refresh.
var ErrNotFound = errors.New("storage: not found")

// CompetitionsRepo persists and queries Competition rows.
type CompetitionsRepo interface {
	UpsertCompetition(ctx context.Context, c models.Competition) error
	ListCompetitions(ctx context.Context) ([]models.Competition, error)
	// GetCompetition returns ErrNotFound if no such row is cached.
	GetCompetition(ctx context.Context, id string) (models.Competition, error)
	// FindCompetitionsByName matches name substrings, case-insensitively.
	FindCompetitionsByName(ctx context.Context, query string) ([]models.Competition, error)
	GetCompetitions(ctx context.Context, ids []string) (map[string]models.Competition, error)
}

// SeasonsRepo persists and queries Season rows.
type SeasonsRepo interface {
	UpsertSeason(ctx context.Context, s models.Season) error
	// ListSeasons returns a competition's cached seasons, most recent first.
	ListSeasons(ctx context.Context, competitionID string) ([]models.Season, error)
	GetSeasons(ctx context.Context, ids []string) (map[string]models.Season, error)
}

// TeamsRepo persists and queries Team rows.
type TeamsRepo interface {
	UpsertTeam(ctx context.Context, t models.Team) error
	FindTeamsByName(ctx context.Context, query string) ([]models.Team, error)
	// GetTeam returns one team by ID, or ErrNotFound.
	GetTeam(ctx context.Context, id string) (models.Team, error)
	GetTeams(ctx context.Context, ids []string) (map[string]models.Team, error)
	// ListTeams returns every cached team, ordered by ID.
	ListTeams(ctx context.Context) ([]models.Team, error)
	// NationalTeams returns national teams of one gender ("men", "women";
	// empty for both), ordered by name.
	NationalTeams(ctx context.Context, gender string) ([]models.Team, error)
}

// CacheRepo answers questions about the cache as a whole.
type CacheRepo interface {
	// CacheSummary reports what is cached per competition, ordered by
	// country and tier.
	CacheSummary(ctx context.Context) ([]models.CompetitionCache, error)
}

// SyncLogRepo records refresh attempts and answers staleness questions.
type SyncLogRepo interface {
	RecordSyncRun(ctx context.Context, r models.SyncRun) error
	// LatestSyncRun returns ErrNotFound if the season was never attempted.
	LatestSyncRun(ctx context.Context, competitionID, seasonID string) (models.SyncRun, error)
	// LatestYearRun returns the latest fetch of one calendar year of a
	// competition's matches, or ErrNotFound.
	LatestYearRun(ctx context.Context, competitionID string, year int) (models.SyncRun, error)
	LatestYearRuns(ctx context.Context) ([]models.SyncRun, error)
	// LastRateLimited returns when a provider last rate-limited footui, or
	// ErrNotFound if it never has.
	LastRateLimited(ctx context.Context) (time.Time, error)
}

// MatchesRepo persists and queries individual matches.
type MatchesRepo interface {
	// UpsertMatches writes matches and their stats in one transaction.
	// Matches are never deleted; stats a later fetch lacks are kept.
	UpsertMatches(ctx context.Context, matches []models.Match) error
	// MatchesForTeam returns matches in the given years (nil for all),
	// newest first, with stats.
	MatchesForTeam(ctx context.Context, teamID string, filter models.MatchFilter, years []int) ([]models.Match, error)
	// MatchesForCompetitionSeason returns matches in kickoff order.
	MatchesForCompetitionSeason(ctx context.Context, competitionID, seasonID string) ([]models.Match, error)
	// RecentMatches returns a team's last limit finished matches, newest
	// first.
	RecentMatches(ctx context.Context, teamID string, limit int) ([]models.Match, error)
	// CountSeasonMatches returns zero for a league season cached before
	// club matches were, which marks it as needing a refetch.
	CountSeasonMatches(ctx context.Context, competitionID, seasonID string) (int, error)
	// TeamMatchSummary is a team's record over what MatchesForTeam returns.
	TeamMatchSummary(ctx context.Context, teamID string, filter models.MatchFilter, years []int) (models.MatchSummary, error)
	// TeamMatchYears returns the years a team has matches in, newest first.
	TeamMatchYears(ctx context.Context, teamID string) ([]int, error)
	// MatchesKickedOff counts matches last stored as scheduled or live that
	// kick off in (since, until]: a sign results are waiting.
	MatchesKickedOff(ctx context.Context, competitionID string, year int, since, until time.Time) (int, error)
	// GetTeamMatchFilter returns the zero filter when none was saved.
	GetTeamMatchFilter(ctx context.Context, teamID string) (models.MatchFilter, error)
	SetTeamMatchFilter(ctx context.Context, teamID string, f models.MatchFilter) error
}

// MatchDetailRepo persists one match's events, full stats, lineups and
// commentary.
type MatchDetailRepo interface {
	// PutMatchDetail replaces everything stored for d.MatchID; the match
	// itself must already be stored.
	PutMatchDetail(ctx context.Context, d models.MatchDetail) error
	// MatchDetail returns ErrNotFound when none was fetched.
	MatchDetail(ctx context.Context, matchID string) (models.MatchDetail, error)
}

// TrophiesRepo persists and queries a club's honours.
type TrophiesRepo interface {
	InsertTrophy(ctx context.Context, t models.Trophy) error
	// DeleteTrophiesBySource lets a re-seed replace rows, not duplicate them.
	DeleteTrophiesBySource(ctx context.Context, teamID, source string) error
	// TeamTrophies returns a team's honours, most recent season first.
	TeamTrophies(ctx context.Context, teamID string) ([]models.Trophy, error)
}

// TeamColorsRepo persists and queries a club's brand colours.
type TeamColorsRepo interface {
	UpsertTeamColors(ctx context.Context, c models.TeamColors) error
	// GetTeamColors returns ErrNotFound for an uncurated club (expected).
	GetTeamColors(ctx context.Context, teamID string) (models.TeamColors, error)
	// TeamColorsFor omits teams without colours.
	TeamColorsFor(ctx context.Context, teamIDs []string) (map[string]models.TeamColors, error)
}

// TeamStatsRepo persists and queries TeamSeasonStats rows and the
// team_competition_season links that scope them per competition.
type TeamStatsRepo interface {
	// ClearCompetitionSeason removes a season's table, squad lines and
	// links, so a refresh replaces the season rather than adding to it.
	ClearCompetitionSeason(ctx context.Context, competitionID, seasonID string) error
	LinkTeamCompetitionSeason(ctx context.Context, teamID, competitionID, seasonID string) error
	UpsertTeamSeasonStats(ctx context.Context, s models.TeamSeasonStats) error
	Standings(ctx context.Context, competitionID, seasonID string) ([]models.TeamSeasonStats, error)
	// TeamHistory returns one row per (competition, season), most recent
	// first; callers group them.
	TeamHistory(ctx context.Context, teamID string) ([]models.TeamSeasonStats, error)
	// TeamSeasonStatExtras returns the long-tail key/value stats, or an
	// empty map. Kept out of Standings to avoid per-row lookups.
	TeamSeasonStatExtras(ctx context.Context, teamID, competitionID, seasonID string) (map[string]float64, error)
	// SeasonStatExtra returns one extra stat keyed by team ID; teams
	// without it are absent.
	SeasonStatExtra(ctx context.Context, competitionID, seasonID, key string) (map[string]float64, error)
}

// PlayersRepo persists and queries Player rows.
type PlayersRepo interface {
	UpsertPlayer(ctx context.Context, p models.Player) error
	// GetPlayer returns one player by ID, or ErrNotFound.
	GetPlayer(ctx context.Context, id string) (models.Player, error)
	GetPlayers(ctx context.Context, ids []string) (map[string]models.Player, error)
	FindPlayersByName(ctx context.Context, query string) ([]models.Player, error)
}

// PlayerStatsRepo persists and queries PlayerSeasonStats rows.
type PlayerStatsRepo interface {
	UpsertPlayerSeasonStats(ctx context.Context, s models.PlayerSeasonStats) error
	RosterForTeamSeason(ctx context.Context, teamID, competitionID, seasonID string) ([]models.PlayerSeasonStats, error)
	// PlayerHistory returns a player's season lines, most recent first.
	PlayerHistory(ctx context.Context, playerID string) ([]models.PlayerSeasonStats, error)
}

// PlayerDetailRepo persists per-player season detail.
type PlayerDetailRepo interface {
	UpsertPlayerSeasonDetail(ctx context.Context, d models.PlayerSeasonDetail) error
	// PlayerSeasonDetails returns detail rows, newest season first.
	PlayerSeasonDetails(ctx context.Context, playerID string) ([]models.PlayerSeasonDetail, error)
	// SeasonPlayerLines returns every player's line in a competition-season.
	SeasonPlayerLines(ctx context.Context, competitionID, seasonID string) ([]models.PlayerSeasonStats, error)
}

// ExternalRefRepo maps provider IDs to footui's canonical IDs.
type ExternalRefRepo interface {
	// ExternalRef returns the canonical ID for a provider ID, or
	// ErrNotFound. entityType is "team" or "player".
	ExternalRef(ctx context.Context, entityType, provider, providerID string) (string, error)
	// ProviderID is the reverse of ExternalRef, or ErrNotFound.
	ProviderID(ctx context.Context, entityType, canonicalID, provider string) (string, error)
	PutExternalRef(ctx context.Context, entityType, canonicalID, provider, providerID string) error
	// TeamPlayers returns every player with a stat line for the team.
	TeamPlayers(ctx context.Context, teamID string) ([]models.Player, error)
}

// FavoritesRepo persists the entities the user has starred.
type FavoritesRepo interface {
	// AddFavorite is a no-op for an existing favourite (keeps AddedAt).
	AddFavorite(ctx context.Context, f models.Favorite) error
	// RemoveFavorite is not an error for a non-favourite.
	RemoveFavorite(ctx context.Context, kind models.FavoriteKind, id string) error
	// ListFavorites returns every favourite, oldest first.
	ListFavorites(ctx context.Context) ([]models.Favorite, error)
}

// Repositories bundles every repo interface.
type Repositories interface {
	CompetitionsRepo
	SeasonsRepo
	TeamsRepo
	TeamStatsRepo
	PlayersRepo
	PlayerStatsRepo
	PlayerDetailRepo
	TeamColorsRepo
	TrophiesRepo
	SyncLogRepo
	CacheRepo
	ExternalRefRepo
	FavoritesRepo
	MatchesRepo
	MatchDetailRepo
}
