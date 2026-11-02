// Package storagetest provides an in-memory SQLite store and seed helpers
// for tests; it uses the real store so screen tests exercise real SQL.
package storagetest

import (
	"context"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

// New returns an empty, migrated in-memory store, closed when the test ends.
func New(t *testing.T) storage.Repositories {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("storagetest: open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return sqlite.NewStore(db)
}

// Fixed is the timestamp on every seeded row, keeping golden output stable.
var Fixed = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// TeamRow is one team's line in a seeded standings table.
type TeamRow struct {
	ID   string
	Name string
	// Position zero leaves it null.
	Position int
	MP       int
	W, D, L  int
	GF, GA   int
	Pts      int
	XG, XGA  float64
	// NoXG leaves xG/xGA null.
	NoXG bool
	// Group is the group or conference, empty for none.
	Group string
}

// SeedCompetition upserts a competition, one season, and a standings
// table, returning the season ID.
func SeedCompetition(t *testing.T, repos storage.Repositories, comp models.Competition, seasonLabel string, rows []TeamRow) string {
	t.Helper()
	ctx := context.Background()
	seasonID := comp.ID + ":" + seasonLabel

	if err := repos.UpsertCompetition(ctx, comp); err != nil {
		t.Fatalf("storagetest: upsert competition: %v", err)
	}
	if err := repos.UpsertSeason(ctx, models.Season{
		ID: seasonID, CompetitionID: comp.ID, Label: seasonLabel,
	}); err != nil {
		t.Fatalf("storagetest: upsert season: %v", err)
	}

	for _, r := range rows {
		if err := repos.UpsertTeam(ctx, models.Team{
			ID: r.ID, Name: r.Name, Country: comp.Country, Gender: comp.Gender,
		}); err != nil {
			t.Fatalf("storagetest: upsert team %s: %v", r.ID, err)
		}
		if err := repos.LinkTeamCompetitionSeason(ctx, r.ID, comp.ID, seasonID); err != nil {
			t.Fatalf("storagetest: link %s: %v", r.ID, err)
		}

		stats := models.TeamSeasonStats{
			TeamID: r.ID, CompetitionID: comp.ID, SeasonID: seasonID,
			MatchesPlayed: r.MP, Wins: r.W, Draws: r.D, Losses: r.L,
			GoalsFor: r.GF, GoalsAgainst: r.GA, GoalDiff: r.GF - r.GA,
			Points: r.Pts, Source: "test", UpdatedAt: Fixed, GroupLabel: r.Group,
		}
		if r.Position != 0 {
			pos := r.Position
			stats.Position = &pos
		}
		if !r.NoXG {
			xg, xga := r.XG, r.XGA
			stats.XG, stats.XGA = &xg, &xga
		}
		if err := repos.UpsertTeamSeasonStats(ctx, stats); err != nil {
			t.Fatalf("storagetest: upsert stats %s: %v", r.ID, err)
		}
	}

	return seasonID
}

// PremierLeague is the competition most screen tests seed against.
func PremierLeague() models.Competition {
	return models.Competition{
		ID: "eng-premier-league", Name: "Premier League",
		Country: "England", Tier: 1, Gender: "men", Format: "league",
	}
}

// SampleStandings covers +/- goal difference, over- and under-performing
// xG, and a row with no xG.
func SampleStandings() []TeamRow {
	return []TeamRow{
		{ID: "eng-arsenal", Name: "Arsenal", Position: 1, MP: 38, W: 28, D: 6, L: 4, GF: 91, GA: 29, Pts: 90, XG: 84.2, XGA: 33.7},
		{ID: "eng-manchester-city", Name: "Manchester City", Position: 2, MP: 38, W: 27, D: 7, L: 4, GF: 96, GA: 34, Pts: 88, XG: 89.9, XGA: 31.2},
		{ID: "eng-wolverhampton-wanderers", Name: "Wolverhampton Wanderers", Position: 3, MP: 38, W: 13, D: 9, L: 16, GF: 50, GA: 65, Pts: 48, XG: 52.4, XGA: 61.8},
		{ID: "eng-luton-town", Name: "Luton Town", Position: 4, MP: 38, W: 6, D: 8, L: 24, GF: 39, GA: 85, Pts: 26, NoXG: true},
	}
}

// PlayerRow is one player's line in a seeded squad.
type PlayerRow struct {
	ID       string
	Name     string
	Position string
	Apps     int
	Minutes  int
	Goals    int
	Assists  int
	XG, XA   float64
}

// SeedRoster upserts players and their season stats for one
// team-competition-season.
func SeedRoster(t *testing.T, repos storage.Repositories, teamID, competitionID, seasonID string, players []PlayerRow) {
	t.Helper()
	ctx := context.Background()

	for _, p := range players {
		if err := repos.UpsertPlayer(ctx, models.Player{
			ID: p.ID, Name: p.Name, Position: p.Position, Gender: "men",
		}); err != nil {
			t.Fatalf("storagetest: upsert player %s: %v", p.ID, err)
		}
		xg, xa := p.XG, p.XA
		if err := repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
			PlayerID: p.ID, TeamID: teamID, CompetitionID: competitionID, SeasonID: seasonID,
			Appearances: p.Apps, Minutes: p.Minutes, Goals: p.Goals, Assists: p.Assists,
			XG: &xg, XA: &xa, Source: "test", UpdatedAt: Fixed,
		}); err != nil {
			t.Fatalf("storagetest: upsert player stats %s: %v", p.ID, err)
		}
	}
}

// ChampionsLeague is a second competition, for proving stats stay per
// competition rather than blended.
func ChampionsLeague() models.Competition {
	return models.Competition{
		ID: "uefa-champions-league", Name: "Champions League",
		Country: "Europe", Tier: 1, Gender: "men", Format: "cup",
	}
}

// SampleSquad is a small deterministic squad.
func SampleSquad() []PlayerRow {
	return []PlayerRow{
		{ID: "p-saka", Name: "Bukayo Saka", Position: "F M S", Apps: 31, Minutes: 2239, Goals: 7, Assists: 5, XG: 8.7, XA: 8.6},
		{ID: "p-rice", Name: "Declan Rice", Position: "D M S", Apps: 36, Minutes: 3111, Goals: 4, Assists: 5, XG: 3.7, XA: 8.2},
		{ID: "p-raya", Name: "David Raya", Position: "GK", Apps: 37, Minutes: 3330, Goals: 0, Assists: 0, XG: 0.0, XA: 0.1},
	}
}

// SampleMatchDetail is a home 0–2 away game in full: two away goals (one
// a penalty), a booking each side, a sub, both whistles, short lineups,
// a few stats and commentary.
func SampleMatchDetail(matchID string) models.MatchDetail {
	n := func(v int) *int { return &v }
	f := func(v float64) *float64 { return &v }
	return models.MatchDetail{
		MatchID: matchID, Final: true, Venue: "Northbridge Park", Attendance: 41250, FetchedAt: Fixed,
		Home: models.MatchSide{
			Formation: "4-3-3",
			Stats: models.MatchDetailStats{Possession: f(44), Shots: n(6), ShotsOnTarget: n(1), Passes: n(380),
				AccuratePasses: n(300), Corners: n(3), YellowCards: n(1)},
			Lineup: []models.LineupPlayer{
				{Name: "Tom Keeper", Shirt: "1", Position: "G", Place: 1, Starter: true},
				{Name: "Joe Defender", Shirt: "4", Position: "CD", Place: 4, Starter: true, Yellow: true, Off: 70},
				{Name: "Sam Bench", Shirt: "14", Position: "M", On: 70},
				{Name: "Unused Ulf", Shirt: "30", Position: "G"},
			},
		},
		Away: models.MatchSide{
			Formation: "4-2-3-1",
			Stats: models.MatchDetailStats{Possession: f(56), Shots: n(17), ShotsOnTarget: n(5), Passes: n(510),
				AccuratePasses: n(460), Corners: n(8), YellowCards: n(1)},
			Lineup: []models.LineupPlayer{
				{Name: "David Raya", Shirt: "22", Position: "G", Place: 1, Starter: true},
				{Name: "Kai Havertz", Shirt: "29", Position: "F", Place: 11, Starter: true, Goals: 1},
				{Name: "Bukayo Saka", Shirt: "7", Position: "RW", Place: 10, Starter: true, Goals: 1, Assists: 1},
			},
		},
		Events: []models.MatchEvent{
			{Kind: models.EventGoal, Side: models.SideAway, Period: 1, Minute: 16, Player: "Kai Havertz", Other: "Bukayo Saka"},
			{Kind: models.EventYellow, Side: models.SideHome, Period: 1, Minute: 33, Player: "Joe Defender"},
			{Kind: models.EventHalfTime, Period: 1, Minute: 45, Added: 2},
			{Kind: models.EventPenaltyGoal, Side: models.SideAway, Period: 2, Minute: 54, Player: "Bukayo Saka"},
			{Kind: models.EventSub, Side: models.SideHome, Period: 2, Minute: 70, Player: "Sam Bench", Other: "Joe Defender"},
			{Kind: models.EventYellow, Side: models.SideAway, Period: 2, Minute: 81, Player: "David Raya"},
			{Kind: models.EventFullTime, Period: 2, Minute: 90, Added: 4},
		},
		Commentary: []models.CommentaryLine{
			{Text: "Lineups are announced and players are warming up."},
			{Clock: "16'", Kind: "Goal", Text: "Goal! Northbridge 0, Arsenal 1. Kai Havertz (Arsenal) header from the centre of the box."},
			{Clock: "54'", Kind: "Penalty - Scored", Text: "Goal! Northbridge 0, Arsenal 2. Bukayo Saka (Arsenal) converts the penalty."},
		},
	}
}
