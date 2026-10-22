// Package models defines the canonical data model shared by providers,
// storage and the TUI. No provider-specific type escapes its package.
package models

import "time"

// Competition is a single league/cup (e.g. English Premier League).
type Competition struct {
	ID      string // slug, e.g. "eng-premier-league"
	Name    string
	Country string
	Tier    int    // 1 = top flight, 2 = second division
	Gender  string // "men" | "women"
	Format  string // "league" | "cup", defaults to "league"
	// CalendarYear: seasons fall within one calendar year (MLS, NWSL).
	CalendarYear bool
	// Category is empty for a club league, CategoryCup for a domestic
	// cup, else one of the international categories.
	Category string
	// ShortName is a compact label ("WC", "UNL"); may be empty.
	ShortName string
}

// International competition categories.
const (
	CategoryTournament    = "tournament"
	CategoryQualifier     = "qualifier"
	CategoryNationsLeague = "nations-league"
	CategoryFriendly      = "friendly"
)

// CategoryCup is a domestic club cup: a source of matches, not a table.
const CategoryCup = "cup"

// International reports whether this is a competition between national
// teams.
func (c Competition) International() bool { return c.Category != "" && c.Category != CategoryCup }

// Cup reports whether this is a domestic club cup.
func (c Competition) Cup() bool { return c.Category == CategoryCup }

// HasTable reports whether an international competition has group tables.
func (c Competition) HasTable() bool {
	return c.International() && c.Category != CategoryFriendly
}

// Abbrev is ShortName, or Name when there is no short form.
func (c Competition) Abbrev() string {
	if c.ShortName != "" {
		return c.ShortName
	}
	return c.Name
}

// Season is one edition of a Competition, keyed by its start year: the
// 2025-26 Premier League is "2025".
type Season struct {
	ID            string // "eng-premier-league:2025"
	CompetitionID string
	Label         string // the start year, "2025"
	StartDate     *time.Time
	EndDate       *time.Time
	// CalendarYear is copied from the competition when read.
	CalendarYear bool
}

// Team is a club or national side. Men's and women's sides are separate
// Teams with distinct IDs.
type Team struct {
	ID          string // slug, e.g. "eng-manchester-united"
	Name        string
	ShortName   string
	Country     string
	Gender      string // "men" | "women"
	FoundedYear int
	Venue       string
	// National marks a national team; its ID is "nat-{slug}[-women]".
	National bool
}

// Label is the team's name, with "(women)" for a women's national team.
func (t Team) Label() string {
	if t.National && t.Gender == "women" {
		return t.Name + " (women)"
	}
	return t.Name
}

// TeamSeasonStats is a team's aggregate for one Competition in one Season;
// numbers are never blended across competitions.
type TeamSeasonStats struct {
	TeamID        string
	CompetitionID string
	SeasonID      string
	// GroupLabel is the tournament group ("Group A"); Position ranks
	// within it. Empty for a single table.
	GroupLabel     string
	Position       *int // league standing rank; nil for cup competitions
	MatchesPlayed  int
	Wins           int
	Draws          int
	Losses         int
	GoalsFor       int
	GoalsAgainst   int
	GoalDiff       int
	Points         int
	PossessionAvg  *float64
	FoulsCommitted *int
	FoulsSuffered  *int
	YellowCards    *int
	RedCards       *int
	Shots          *int
	ShotsOnTarget  *int
	Corners        *int
	XG             *float64 // nullable: only populated once an xG pass has run
	XGA            *float64
	// Extras holds long-tail stats keyed by the Stat* constants; nil when
	// a provider has none.
	Extras    map[string]float64
	Source    string // provenance, e.g. "understat"
	UpdatedAt time.Time
}

// Long-tail team stat keys for TeamSeasonStats.Extras. Storage and the
// TUI read them too, so they live here.
const (
	// StatNPxG is non-penalty expected goals.
	StatNPxG = "npxg"
	// StatNPxGA is non-penalty expected goals against.
	StatNPxGA = "npxga"
	// StatPPDA is opponent passes allowed per defensive action: lower
	// means a more intense press.
	StatPPDA = "ppda"
	// StatPPDAAllowed is the same measure applied to the opponent's press.
	StatPPDAAllowed = "ppda_allowed"
	// StatDeep counts completed passes within ~20 yards of goal, excluding
	// crosses.
	StatDeep = "deep"
	// StatDeepAllowed is the same, conceded.
	StatDeepAllowed = "deep_allowed"
	// StatExpectedPoints is points expected from each match's chances.
	StatExpectedPoints = "xpts"
)

// Player is a person who has appeared for at least one in-scope Team.
type Player struct {
	ID          string
	Name        string
	DateOfBirth *time.Time
	Nationality string
	Position    string
	Gender      string
}

// PlayerSeasonStats is a player's aggregate for one Team in one
// Competition in one Season.
type PlayerSeasonStats struct {
	PlayerID      string
	TeamID        string
	CompetitionID string
	SeasonID      string
	Appearances   int
	Minutes       int
	Goals         int
	Assists       int
	XG            *float64
	XA            *float64
	// Nullable: a provider may supply the basics without these.
	Shots           *int
	KeyPasses       *int
	NonPenaltyGoals *int
	NPxG            *float64
	YellowCards     int
	RedCards        int
	Source          string
	UpdatedAt       time.Time
}

// PlayerSeasonDetail is the positional rest of a player's season for one
// Team in one Competition, fetched on demand per player. nil means "not
// recorded", which a provider's zero sometimes really means.
type PlayerSeasonDetail struct {
	PlayerID      string
	TeamID        string
	CompetitionID string
	SeasonID      string
	// Position is "G", "D", "M", "F" or "".
	Position      string
	Appearances   *int
	Minutes       *int
	CleanSheets   *int
	GoalsConceded *int
	Saves         *int
	Tackles       *int
	Interceptions *int
	Clearances    *int
	ShotsOnTarget *int
	// PassPct is the share of passes completed, 0 to 1.
	PassPct   *float64
	FetchedAt time.Time
}

// Trophy is a single honour in a Team's history. SeasonLabel is free text,
// since trophies pre-date most in-scope seasons.
type Trophy struct {
	ID            int64
	TeamID        string
	CompetitionID string // optional: nil/empty when the cup isn't itself an in-scope Competition
	Title         string
	SeasonLabel   string
	TrophyType    string // "league_title" | "domestic_cup" | "continental_cup" | "other"
	Source        string
}

// SyncRun records one attempt to refresh one competition-season. Failures
// are recorded too, so staleness reflects attempts, not just successes.
type SyncRun struct {
	ID            int64
	Provider      string
	CompetitionID string
	SeasonID      string
	StartedAt     time.Time
	FinishedAt    time.Time
	Status        string // SyncStatus* below
	RowsUpserted  int
	Error         string
	// Year is set for an international calendar-year run; SeasonID is
	// then empty.
	Year int
}

// Sync run statuses.
const (
	SyncStatusOK = "ok"
	// SyncStatusFailed is a unit that was attempted and errored.
	SyncStatusFailed = "failed"
	// SyncStatusSkipped is a unit never attempted after the run gave up.
	SyncStatusSkipped = "skipped"
	// SyncStatusRateLimited counts as a failure and suspends automatic
	// refreshes for a while.
	SyncStatusRateLimited = "rate_limited"
)

// Succeeded reports whether the run completed without error.
func (r SyncRun) Succeeded() bool { return r.Status == SyncStatusOK }

// CompetitionCache summarizes how much is cached for one competition and
// how fresh it is.
type CompetitionCache struct {
	Competition Competition
	Seasons     int
	// EarliestSeason and LatestSeason are season labels, empty when
	// nothing is cached.
	EarliestSeason string
	LatestSeason   string
	Teams          int
	// PlayerSeasons counts player-season rows, not distinct players.
	PlayerSeasons int
	// LastSync is the latest attempt across all seasons; nil if never.
	LastSync *SyncRun
}

// TeamColors are the brand colours used to theme a Team's dashboard.
type TeamColors struct {
	TeamID       string
	PrimaryHex   string
	SecondaryHex string
	TertiaryHex  string
	Source       string
}

// FavoriteKind is the kind of entity a Favorite points at.
type FavoriteKind string

// Favorite kinds. The values are what the favorite table stores.
const (
	FavoriteCompetition FavoriteKind = "competition"
	FavoriteTeam        FavoriteKind = "team"
	FavoritePlayer      FavoriteKind = "player"
)

// Favorite is a starred competition, team or player, keyed by canonical
// ID so it holds across competitions.
type Favorite struct {
	Kind    FavoriteKind
	ID      string
	AddedAt time.Time
}

// FoundPlayer is a player a data source's search turned up.
type FoundPlayer struct {
	// Provider and ID together form the player's external_ref.
	Provider string
	ID       string
	Name     string
	// Context is the source's description, e.g. "English Premier League -
	// Man City".
	Context string
}

// CareerSeason is one competition-season a player appeared in. Year is
// the season's start year.
type CareerSeason struct {
	CompetitionID string
	Year          string
}
