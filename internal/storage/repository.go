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
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// ErrNotFound is returned by the single-row Get* queries when nothing is
// cached for the requested ID. Callers distinguish "no data cached yet,
// tell the user to run footui refresh" from a real backend failure with
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
	// query (case-insensitive), for resolving `footui league <name>`.
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
	// NationalTeams returns the cached national teams of one gender ("men"
	// or "women"; empty for both), ordered by name.
	NationalTeams(ctx context.Context, gender string) ([]models.Team, error)
}

// CacheRepo answers questions about the cache as a whole rather than about
// one entity.
type CacheRepo interface {
	// CacheSummary reports what is cached per competition, ordered by
	// country and tier.
	CacheSummary(ctx context.Context) ([]models.CompetitionCache, error)
}

// SyncLogRepo records refresh attempts and answers staleness questions.
type SyncLogRepo interface {
	RecordSyncRun(ctx context.Context, r models.SyncRun) error
	// LatestSyncRun returns the most recent attempt for one
	// competition-season, or ErrNotFound if it has never been attempted.
	LatestSyncRun(ctx context.Context, competitionID, seasonID string) (models.SyncRun, error)
	// LatestYearRun returns the most recent fetch of one calendar year of
	// a competition's matches, or ErrNotFound.
	LatestYearRun(ctx context.Context, competitionID string, year int) (models.SyncRun, error)
	// LatestYearRuns returns the most recent fetch of every
	// competition-year ever attempted.
	LatestYearRuns(ctx context.Context) ([]models.SyncRun, error)
	// LastRateLimited is when a provider last turned footui away for
	// asking too often, or ErrNotFound if it never has.
	LastRateLimited(ctx context.Context) (time.Time, error)
}

// MatchesRepo persists and queries individual matches — the grain
// national teams are tracked at, and a club's recent games come from.
type MatchesRepo interface {
	// UpsertMatches writes matches and their per-team stats in one
	// transaction, by match ID. Matches are never deleted, so a
	// rescheduled fixture updates in place; stats a later fetch lacks are
	// kept rather than blanked.
	UpsertMatches(ctx context.Context, matches []models.Match) error
	// MatchesForTeam returns a team's matches that pass filter, in the
	// given calendar years (nil for all), newest first, with stats.
	MatchesForTeam(ctx context.Context, teamID string, filter models.MatchFilter, years []int) ([]models.Match, error)
	// MatchesForCompetitionSeason returns every match of one edition of a
	// competition, in kickoff order.
	MatchesForCompetitionSeason(ctx context.Context, competitionID, seasonID string) ([]models.Match, error)
	// RecentMatches returns a team's last limit finished matches in every
	// competition, newest first, with stats.
	RecentMatches(ctx context.Context, teamID string, limit int) ([]models.Match, error)
	// CountSeasonMatches counts the matches stored for one season of a
	// competition: none for a league season cached before club matches
	// were, which is how that season is known to need fetching again.
	CountSeasonMatches(ctx context.Context, competitionID, seasonID string) (int, error)
	// TeamMatchSummary is a team's record over the matches MatchesForTeam
	// would return.
	TeamMatchSummary(ctx context.Context, teamID string, filter models.MatchFilter, years []int) (models.MatchSummary, error)
	// TeamMatchYears returns the calendar years a team has matches in,
	// newest first.
	TeamMatchYears(ctx context.Context, teamID string) ([]int, error)
	// MatchesKickedOff counts a competition-year's matches that were last
	// stored as scheduled or in play and kick off in (since, until] — the
	// sign that a stored year has results waiting.
	MatchesKickedOff(ctx context.Context, competitionID string, year int, since, until time.Time) (int, error)
	// GetTeamMatchFilter returns the filter last used on a team, or the
	// zero filter (every match) when none was saved.
	GetTeamMatchFilter(ctx context.Context, teamID string) (models.MatchFilter, error)
	SetTeamMatchFilter(ctx context.Context, teamID string, f models.MatchFilter) error
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

// TeamColorsRepo persists and queries a club's brand colours.
type TeamColorsRepo interface {
	UpsertTeamColors(ctx context.Context, c models.TeamColors) error
	// GetTeamColors returns a team's colours, or ErrNotFound when none are
	// seeded — a normal, expected state for a club not yet curated.
	GetTeamColors(ctx context.Context, teamID string) (models.TeamColors, error)
	// TeamColorsFor returns the colours of every listed team that has any,
	// keyed by team ID, in one query. Teams without colours are absent.
	TeamColorsFor(ctx context.Context, teamIDs []string) (map[string]models.TeamColors, error)
}

// TeamStatsRepo persists and queries TeamSeasonStats rows, and the
// team_competition_season anchor rows that make them queryable per
// competition rather than blended across a team's whole season.
type TeamStatsRepo interface {
	// ClearCompetitionSeason removes one competition-season's table,
	// squad lines and the links that place clubs in it, so a refresh
	// replaces the season rather than adding to it: a club resolved to the
	// wrong canonical one and since corrected leaves no row behind.
	ClearCompetitionSeason(ctx context.Context, competitionID, seasonID string) error
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
	// SeasonStatExtra returns one long-tail stat for every team in a
	// competition-season, keyed by team ID; teams without it are absent.
	SeasonStatExtra(ctx context.Context, competitionID, seasonID, key string) (map[string]float64, error)
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

// PlayerDetailRepo persists the per-player season detail fetched when a
// player is opened or starred.
type PlayerDetailRepo interface {
	UpsertPlayerSeasonDetail(ctx context.Context, d models.PlayerSeasonDetail) error
	// PlayerSeasonDetails returns every detail row for the player, newest
	// season first.
	PlayerSeasonDetails(ctx context.Context, playerID string) ([]models.PlayerSeasonDetail, error)
	// SeasonPlayerLines returns every player's stat line in one
	// competition-season: the baseline a player is compared against.
	SeasonPlayerLines(ctx context.Context, competitionID, seasonID string) ([]models.PlayerSeasonStats, error)
}

// ExternalRefRepo maps a provider's own IDs to footui's canonical ones, so
// the same club or player fetched from two providers is one entity.
type ExternalRefRepo interface {
	// ExternalRef returns the canonical ID a provider's ID was resolved
	// to, or ErrNotFound if it never has been. entityType is "team" or
	// "player".
	ExternalRef(ctx context.Context, entityType, provider, providerID string) (string, error)
	// ProviderID is the reverse: the provider's ID for a canonical
	// entity, or ErrNotFound if it was never resolved from that provider.
	ProviderID(ctx context.Context, entityType, canonicalID, provider string) (string, error)
	PutExternalRef(ctx context.Context, entityType, canonicalID, provider, providerID string) error
	// TeamPlayers returns every player with a stat line for the team, in
	// any competition or season — the squad a newly fetched player is
	// matched against.
	TeamPlayers(ctx context.Context, teamID string) ([]models.Player, error)
}

// FavoritesRepo persists the entities the user has starred.
type FavoritesRepo interface {
	// AddFavorite stars an entity; starring an existing favourite is a
	// no-op that keeps its original AddedAt.
	AddFavorite(ctx context.Context, f models.Favorite) error
	// RemoveFavorite un-stars an entity; removing a non-favourite is not
	// an error.
	RemoveFavorite(ctx context.Context, kind models.FavoriteKind, id string) error
	// ListFavorites returns every favourite, oldest first.
	ListFavorites(ctx context.Context) ([]models.Favorite, error)
}

// Repositories bundles every repo interface — the shape the sync layer and
// CLI/TUI construction code depend on, so wiring up storage is passing one
// value rather than one per repo.
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
}
