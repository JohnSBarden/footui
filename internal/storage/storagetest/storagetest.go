// Package storagetest provides an in-memory storage.Repositories for tests
// in other packages, plus helpers for seeding it with realistic data.
//
// It hands back the real SQLite store rather than a hand-written fake:
// screen tests then exercise the same SQL the binary runs, so a broken
// query fails a screen test instead of only showing up at runtime.
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

// Fixed is the timestamp every seeded row is stamped with, so golden output
// never depends on when the test ran.
var Fixed = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// TeamRow is one team's line in a seeded standings table.
type TeamRow struct {
	ID   string
	Name string
	// Position is the standings rank. Zero means "leave null", for
	// exercising the missing-value rendering path.
	Position int
	MP       int
	W, D, L  int
	GF, GA   int
	Pts      int
	XG, XGA  float64
	// NoXG seeds the row with null xG/xGA, as a provider without expected
	// goals would.
	NoXG bool
}

// SeedCompetition upserts a competition, one season, and a standings table.
// It returns the season ID.
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
			Points: r.Pts, Source: "test", UpdatedAt: Fixed,
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

// SampleStandings is a small, deterministic standings table covering the
// rendering cases that matter: a positive and a negative goal difference, a
// team that outperformed its xG and one that underperformed, and a row with
// no xG data at all.
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

// ChampionsLeague is a second, non-league competition used to prove that a
// team's stats stay sliceable per competition instead of being blended.
// Tier is 0 because "tier" describes a position in a domestic league
// pyramid and means nothing for a continental cup; screens order unknown
// tiers after real ones, so a club's own league leads the tab bar.
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
