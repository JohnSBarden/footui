// Package models defines the canonical data model shared by every provider,
// the storage layer, and the TUI. A provider's job is to map its own
// source-specific shapes into these structs; nothing outside a provider
// package should ever see a provider-specific type.
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
	// CalendarYear marks a competition whose seasons fall within one
	// calendar year (MLS, NWSL): its 2026 season is "2026", not
	// "2026-27".
	CalendarYear bool
	// Category is empty for a club league. An international one is a
	// CategoryTournament, CategoryQualifier, CategoryNationsLeague or
	// CategoryFriendly, which is what a nation's competition filter
	// presets select on. A domestic cup is CategoryCup.
	Category string
	// ShortName is a compact label for tight columns ("WC", "UNL"); empty
	// when there isn't one.
	ShortName string
}

// International competition categories.
const (
	CategoryTournament    = "tournament"
	CategoryQualifier     = "qualifier"
	CategoryNationsLeague = "nations-league"
	CategoryFriendly      = "friendly"
)

// CategoryCup is a domestic club cup (the FA Cup, the DFB-Pokal): a
// source of a club's matches, not a competition with a table to browse.
const CategoryCup = "cup"

// International reports whether this is a competition between national
// teams.
func (c Competition) International() bool { return c.Category != "" && c.Category != CategoryCup }

// Cup reports whether this is a domestic club cup.
func (c Competition) Cup() bool { return c.Category == CategoryCup }

// HasTable reports whether an international competition publishes group
// tables. Friendlies have none, and nor does a domestic cup.
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

// Season is one edition of a Competition (e.g. 2024-25).
type Season struct {
	ID            string // "eng-premier-league:2024-25"
	CompetitionID string
	Label         string // "2024-25" or "2024" for calendar-year competitions
	StartDate     *time.Time
	EndDate       *time.Time
	// CalendarYear is copied from the season's competition when read, so
	// a season can be displayed without looking its competition up.
	CalendarYear bool
}

// Team is a club side. Men's and women's sides for the same club are
// separate Team rows (distinct IDs), since they play in different
// competitions and have independent squads/history.
type Team struct {
	ID          string // slug, e.g. "eng-manchester-united"
	Name        string
	ShortName   string
	Country     string
	Gender      string // "men" | "women"
	FoundedYear int
	Venue       string
	// National marks a national team rather than a club. Its ID is
	// "nat-{slug}" ("nat-{slug}-women"), which can't collide with a club's.
	National bool
}

// Label is how a sentence refers to the team: its name, with "(women)"
// for a women's national team, whose name is just the country's.
func (t Team) Label() string {
	if t.National && t.Gender == "women" {
		return t.Name + " (women)"
	}
	return t.Name
}

// TeamSeasonStats is a team's aggregate performance in one Competition in
// one Season. This is the row-per-(team,competition,season) grain that
// keeps a team's numbers sliceable by competition instead of blended.
type TeamSeasonStats struct {
	TeamID        string
	CompetitionID string
	SeasonID      string
	// GroupLabel is the group a tournament table row belongs to ("Group
	// A"); Position is then the rank within it. Empty for one table.
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
	// Extras carries source-specific long-tail stats that don't warrant a
	// column of their own — pressing intensity, expected points, deep
	// completions. Keys are stable snake_case identifiers (see
	// models.Stat* constants); a provider that has none leaves this nil,
	// and screens render whichever keys are actually present.
	Extras    map[string]float64
	Source    string // provenance, e.g. "understat"
	UpdatedAt time.Time
}

// Long-tail team stat keys used in TeamSeasonStats.Extras. They are
// declared here rather than in a provider package because the storage
// layer and TUI both read them: a provider chooses which it can supply,
// but not what they are called.
const (
	// StatNPxG is non-penalty expected goals — expected goals with
	// penalties excluded, a better measure of open-play attacking quality
	// than raw xG.
	StatNPxG = "npxg"
	// StatNPxGA is non-penalty expected goals against.
	StatNPxGA = "npxga"
	// StatPPDA is opponent passes allowed per defensive action: lower
	// means a more intense press.
	StatPPDA = "ppda"
	// StatPPDAAllowed is the same measure applied to the opponent's press.
	StatPPDAAllowed = "ppda_allowed"
	// StatDeep counts completed passes within ~20 yards of the opponent's
	// goal (excluding crosses).
	StatDeep = "deep"
	// StatDeepAllowed is the same, conceded.
	StatDeepAllowed = "deep_allowed"
	// StatExpectedPoints is points the team would be expected to have
	// taken given the chances in each match.
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

// PlayerSeasonStats is a player's aggregate performance for one Team in one
// Competition in one Season — same (competition, season) grain as
// TeamSeasonStats, for the same "don't blend across competitions" reason.
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
	// Shots, KeyPasses, NonPenaltyGoals, and NPxG are nullable because a
	// provider may supply the basics without them.
	Shots           *int
	KeyPasses       *int
	NonPenaltyGoals *int
	NPxG            *float64
	YellowCards     int
	RedCards        int
	Source          string
	UpdatedAt       time.Time
}

// PlayerSeasonDetail is the rest of a player's season for one Team in one
// Competition — what a position is judged on beyond goals and assists:
// clean sheets and saves for a keeper, tackles and interceptions for a
// defender. It is fetched per player, on demand, so it exists only for
// players someone has opened or starred. Every stat is nullable: nil is
// "not recorded", which a provider's zero sometimes really means.
type PlayerSeasonDetail struct {
	PlayerID      string
	TeamID        string
	CompetitionID string
	SeasonID      string
	// Position is the provider's single position code — "G", "D", "M"
	// or "F" — or "" when it gave none.
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

// Trophy is a single honour in a Team's history. Not tied to a Season row
// since trophies pre-date most in-scope seasons; SeasonLabel is a free-text
// year/season string for display.
type Trophy struct {
	ID            int64
	TeamID        string
	CompetitionID string // optional: nil/empty when the cup isn't itself an in-scope Competition
	Title         string
	SeasonLabel   string
	TrophyType    string // "league_title" | "domestic_cup" | "continental_cup" | "other"
	Source        string
}

// SyncRun records one attempt to refresh one competition-season. Failed
// attempts are recorded too: "we tried an hour ago and it didn't work" is
// different from "we haven't tried since Tuesday", and a staleness
// indicator that hides failures would show the second when it means the
// first.
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
	// Year is set for a run that fetched one calendar year of a
	// competition's matches (an international "unit") rather than one
	// season; SeasonID is then empty.
	Year int
}

// Sync run statuses.
const (
	SyncStatusOK = "ok"
	// SyncStatusFailed is a unit that was attempted and errored.
	SyncStatusFailed = "failed"
	// SyncStatusSkipped is a unit that was never attempted, because the
	// run gave up after repeated failures.
	SyncStatusSkipped = "skipped"
	// SyncStatusRateLimited is a unit the provider refused because footui
	// was asking too often. It counts as a failure, and it suspends
	// automatic refreshes for a while.
	SyncStatusRateLimited = "rate_limited"
)

// Succeeded reports whether the run completed without error.
func (r SyncRun) Succeeded() bool { return r.Status == SyncStatusOK }

// CompetitionCache summarizes what is cached for one competition: how much
// data is held and how fresh it is. It exists because a tool built around
// a local cache has to be able to answer "what do I actually have?"
// without making the user open the interface and look.
type CompetitionCache struct {
	Competition Competition
	Seasons     int
	// EarliestSeason and LatestSeason are season labels, empty when
	// nothing is cached.
	EarliestSeason string
	LatestSeason   string
	Teams          int
	// PlayerSeasons counts player-season rows, not distinct players: it is
	// a measure of how much was fetched, not of squad size.
	PlayerSeasons int
	// LastSync is the most recent refresh attempt across any of this
	// competition's seasons, or nil if it has never been attempted.
	LastSync *SyncRun
}

// TeamColors are the brand colors used to theme a Team's dashboard.
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

// Favorite is a competition, team or player the user has starred. It is
// keyed by canonical ID, so a club favourited from its Premier League
// page is the same favourite when reached through the Champions League.
type Favorite struct {
	Kind    FavoriteKind
	ID      string
	AddedAt time.Time
}
