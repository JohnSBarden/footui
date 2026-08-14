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
}

// Season is one edition of a Competition (e.g. 2024-25).
type Season struct {
	ID            string // "eng-premier-league:2024-25"
	CompetitionID string
	Label         string // "2024-25" or "2024" for calendar-year competitions
	StartDate     *time.Time
	EndDate       *time.Time
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
}

// TeamSeasonStats is a team's aggregate performance in one Competition in
// one Season. This is the row-per-(team,competition,season) grain that
// keeps a team's numbers sliceable by competition instead of blended.
type TeamSeasonStats struct {
	TeamID         string
	CompetitionID  string
	SeasonID       string
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
	XG             *float64 // nullable: only populated once an XGProvider pass has run
	XGA            *float64
	Source         string // provenance, e.g. "understat"
	UpdatedAt      time.Time
}

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
	YellowCards   int
	RedCards      int
	Source        string
	UpdatedAt     time.Time
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

// TeamColors are the brand colors used to theme a Team's dashboard.
type TeamColors struct {
	TeamID       string
	PrimaryHex   string
	SecondaryHex string
	TertiaryHex  string
	Source       string
}
