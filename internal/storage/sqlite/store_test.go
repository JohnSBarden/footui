package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// newTestStore returns a Store over a fresh in-memory database. Repository
// tests run against real SQLite rather than a fake so that schema
// constraints (foreign keys, the composite team_competition_season key)
// are actually exercised.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

// seedCompetitionSeason upserts a competition, a season, a team, and the
// anchor row linking them — the minimum needed before team_season_stats
// will accept a row.
func seedCompetitionSeason(t *testing.T, s *Store, compID, seasonLabel, teamID, teamName string) string {
	t.Helper()
	ctx := context.Background()
	seasonID := compID + ":" + seasonLabel

	if err := s.UpsertCompetition(ctx, models.Competition{
		ID: compID, Name: "Comp " + compID, Country: "England", Tier: 1, Gender: "men",
	}); err != nil {
		t.Fatalf("UpsertCompetition: %v", err)
	}
	if err := s.UpsertSeason(ctx, models.Season{
		ID: seasonID, CompetitionID: compID, Label: seasonLabel,
	}); err != nil {
		t.Fatalf("UpsertSeason: %v", err)
	}
	if err := s.UpsertTeam(ctx, models.Team{
		ID: teamID, Name: teamName, Country: "England", Gender: "men",
	}); err != nil {
		t.Fatalf("UpsertTeam: %v", err)
	}
	if err := s.LinkTeamCompetitionSeason(ctx, teamID, compID, seasonID); err != nil {
		t.Fatalf("LinkTeamCompetitionSeason: %v", err)
	}
	return seasonID
}

func TestGetCompetitionReturnsNotFoundForUnknownID(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetCompetition(context.Background(), "no-such-comp")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected storage.ErrNotFound, got %v", err)
	}
}

func TestGetCompetitionRoundTrips(t *testing.T) {
	s := newTestStore(t)
	seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")

	got, err := s.GetCompetition(context.Background(), "eng-premier-league")
	if err != nil {
		t.Fatalf("GetCompetition: %v", err)
	}
	if got.Name != "Comp eng-premier-league" || got.Tier != 1 || got.Format != "league" {
		t.Errorf("unexpected competition: %+v", got)
	}
}

func TestFindCompetitionsByNameIsCaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.UpsertCompetition(ctx, models.Competition{
		ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men",
	}); err != nil {
		t.Fatalf("UpsertCompetition: %v", err)
	}

	for _, query := range []string{"premier", "PREMIER", "Premier League"} {
		got, err := s.FindCompetitionsByName(ctx, query)
		if err != nil {
			t.Fatalf("FindCompetitionsByName(%q): %v", query, err)
		}
		if len(got) != 1 || got[0].ID != "eng-premier-league" {
			t.Errorf("FindCompetitionsByName(%q) = %+v, want the Premier League row", query, got)
		}
	}
}

func TestListSeasonsOrdersMostRecentFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedCompetitionSeason(t, s, "eng-premier-league", "2023", "eng-arsenal", "Arsenal")
	for _, label := range []string{"2025", "2024"} {
		if err := s.UpsertSeason(ctx, models.Season{
			ID: "eng-premier-league:" + label, CompetitionID: "eng-premier-league", Label: label,
		}); err != nil {
			t.Fatalf("UpsertSeason %s: %v", label, err)
		}
	}

	got, err := s.ListSeasons(ctx, "eng-premier-league")
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	want := []string{"2025", "2024", "2023"}
	if len(got) != len(want) {
		t.Fatalf("ListSeasons returned %d seasons, want %d", len(got), len(want))
	}
	for i, label := range want {
		if got[i].Label != label {
			t.Errorf("season %d = %q, want %q", i, got[i].Label, label)
		}
	}
}

func TestListSeasonsScopesToOneCompetition(t *testing.T) {
	s := newTestStore(t)
	seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")
	seedCompetitionSeason(t, s, "esp-la-liga", "2025", "esp-barcelona", "Barcelona")

	got, err := s.ListSeasons(context.Background(), "esp-la-liga")
	if err != nil {
		t.Fatalf("ListSeasons: %v", err)
	}
	if len(got) != 1 || got[0].CompetitionID != "esp-la-liga" {
		t.Errorf("ListSeasons leaked another competition's seasons: %+v", got)
	}
}

func TestGetTeamsResolvesBatchAndSkipsUnknownIDs(t *testing.T) {
	s := newTestStore(t)
	seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")
	seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-chelsea", "Chelsea")

	got, err := s.GetTeams(context.Background(), []string{"eng-arsenal", "eng-chelsea", "eng-ghost"})
	if err != nil {
		t.Fatalf("GetTeams: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetTeams returned %d teams, want 2", len(got))
	}
	if got["eng-arsenal"].Name != "Arsenal" || got["eng-chelsea"].Name != "Chelsea" {
		t.Errorf("unexpected teams: %+v", got)
	}
	if _, ok := got["eng-ghost"]; ok {
		t.Error("GetTeams should omit IDs with no cached team row, not invent one")
	}
}

func TestGetTeamsWithNoIDsMakesNoQuery(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetTeams(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetTeams(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetTeams(nil) = %+v, want empty", got)
	}
}

func TestGetTeamReturnsNotFoundForUnknownID(t *testing.T) {
	s := newTestStore(t)

	_, err := s.GetTeam(context.Background(), "eng-ghost")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected storage.ErrNotFound, got %v", err)
	}
}

// Standings is the query the League screen renders, so it must come back
// ranked by the position the mapper computed, not in insertion order.
func TestStandingsOrderByPosition(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seasonID := seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")
	seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-chelsea", "Chelsea")

	for teamID, pos := range map[string]int{"eng-chelsea": 2, "eng-arsenal": 1} {
		p := pos
		if err := s.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{
			TeamID: teamID, CompetitionID: "eng-premier-league", SeasonID: seasonID,
			Position: &p, Source: "test", UpdatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("UpsertTeamSeasonStats %s: %v", teamID, err)
		}
	}

	got, err := s.Standings(ctx, "eng-premier-league", seasonID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if len(got) != 2 || got[0].TeamID != "eng-arsenal" || got[1].TeamID != "eng-chelsea" {
		t.Errorf("standings not ordered by position: %+v", got)
	}
}
