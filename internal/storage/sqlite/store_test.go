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

func TestTeamSeasonStatExtrasRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seasonID := seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")

	err := s.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{
		TeamID: "eng-arsenal", CompetitionID: "eng-premier-league", SeasonID: seasonID,
		Source: "test", UpdatedAt: time.Now(),
		Extras: map[string]float64{
			models.StatPPDA:           10.5,
			models.StatExpectedPoints: 74.2,
			models.StatDeep:           320,
		},
	})
	if err != nil {
		t.Fatalf("UpsertTeamSeasonStats: %v", err)
	}

	got, err := s.TeamSeasonStatExtras(ctx, "eng-arsenal", "eng-premier-league", seasonID)
	if err != nil {
		t.Fatalf("TeamSeasonStatExtras: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d extras, want 3: %v", len(got), got)
	}
	if got[models.StatPPDA] != 10.5 || got[models.StatExpectedPoints] != 74.2 {
		t.Errorf("extras did not round-trip: %v", got)
	}
}

// A second provider supplying only some keys must not wipe the ones
// another provider already contributed for the same team-season.
func TestUpsertingSomeExtrasLeavesOthersIntact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seasonID := seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")

	base := models.TeamSeasonStats{
		TeamID: "eng-arsenal", CompetitionID: "eng-premier-league", SeasonID: seasonID,
		Source: "test", UpdatedAt: time.Now(),
	}

	base.Extras = map[string]float64{models.StatPPDA: 10.5, models.StatDeep: 320}
	if err := s.UpsertTeamSeasonStats(ctx, base); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	base.Extras = map[string]float64{models.StatPPDA: 9.1}
	if err := s.UpsertTeamSeasonStats(ctx, base); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.TeamSeasonStatExtras(ctx, "eng-arsenal", "eng-premier-league", seasonID)
	if err != nil {
		t.Fatalf("TeamSeasonStatExtras: %v", err)
	}
	if got[models.StatPPDA] != 9.1 {
		t.Errorf("PPDA = %v, want the updated 9.1", got[models.StatPPDA])
	}
	if got[models.StatDeep] != 320 {
		t.Errorf("deep = %v, want the untouched 320 — a partial update must not delete other keys", got[models.StatDeep])
	}
}

func TestPlayerAdvancedStatsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seasonID := seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")

	if err := s.UpsertPlayer(ctx, models.Player{ID: "p1", Name: "Bukayo Saka", Gender: "men"}); err != nil {
		t.Fatalf("UpsertPlayer: %v", err)
	}
	shots, keyPasses, npg := 84, 61, 7
	npxg := 8.1
	if err := s.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
		PlayerID: "p1", TeamID: "eng-arsenal", CompetitionID: "eng-premier-league", SeasonID: seasonID,
		Appearances: 31, Minutes: 2239, Goals: 7, Assists: 5,
		Shots: &shots, KeyPasses: &keyPasses, NonPenaltyGoals: &npg, NPxG: &npxg,
		Source: "test", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertPlayerSeasonStats: %v", err)
	}

	roster, err := s.RosterForTeamSeason(ctx, "eng-arsenal", "eng-premier-league", seasonID)
	if err != nil {
		t.Fatalf("RosterForTeamSeason: %v", err)
	}
	if len(roster) != 1 {
		t.Fatalf("got %d roster rows", len(roster))
	}
	got := roster[0]
	if got.Shots == nil || *got.Shots != 84 {
		t.Errorf("shots = %v", got.Shots)
	}
	if got.NPxG == nil || *got.NPxG != 8.1 {
		t.Errorf("npxG = %v", got.NPxG)
	}
}

// A provider without advanced stats must leave them null, not zero.
func TestPlayerAdvancedStatsStayNullWhenNotSupplied(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seasonID := seedCompetitionSeason(t, s, "eng-premier-league", "2025", "eng-arsenal", "Arsenal")

	if err := s.UpsertPlayer(ctx, models.Player{ID: "p1", Name: "Basic Player", Gender: "men"}); err != nil {
		t.Fatalf("UpsertPlayer: %v", err)
	}
	if err := s.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
		PlayerID: "p1", TeamID: "eng-arsenal", CompetitionID: "eng-premier-league", SeasonID: seasonID,
		Appearances: 10, Source: "test", UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("UpsertPlayerSeasonStats: %v", err)
	}

	roster, err := s.RosterForTeamSeason(ctx, "eng-arsenal", "eng-premier-league", seasonID)
	if err != nil {
		t.Fatalf("RosterForTeamSeason: %v", err)
	}
	if roster[0].Shots != nil {
		t.Errorf("shots = %v, want nil when the provider had none", *roster[0].Shots)
	}
}

// Football is full of accented names and people search from an ASCII
// keyboard. Before folding, "martin" could not find "Martín Zubimendi" and
// "jurrien" found nothing at all.
func TestSearchFindsAccentedNamesFromASCIIQueries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, p := range []models.Player{
		{ID: "p1", Name: "Martín Zubimendi", Gender: "men"},
		{ID: "p2", Name: "Jurriën Timber", Gender: "men"},
		{ID: "p3", Name: "Martin Odegaard", Gender: "men"},
	} {
		if err := s.UpsertPlayer(ctx, p); err != nil {
			t.Fatalf("UpsertPlayer: %v", err)
		}
	}

	cases := map[string][]string{
		"martin":    {"Martín Zubimendi", "Martin Odegaard"},
		"MARTIN":    {"Martín Zubimendi", "Martin Odegaard"},
		"Martín":    {"Martín Zubimendi", "Martin Odegaard"},
		"jurrien":   {"Jurriën Timber"},
		"Jurriën":   {"Jurriën Timber"},
		"zubimendi": {"Martín Zubimendi"},
	}
	for query, want := range cases {
		got, err := s.FindPlayersByName(ctx, query)
		if err != nil {
			t.Fatalf("FindPlayersByName(%q): %v", query, err)
		}
		names := map[string]bool{}
		for _, p := range got {
			names[p.Name] = true
		}
		for _, w := range want {
			if !names[w] {
				t.Errorf("FindPlayersByName(%q) missed %q (got %d results)", query, w, len(got))
			}
		}
	}
}

func TestTeamSearchIsAccentInsensitive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, team := range []models.Team{
		{ID: "ger-bayern-munich", Name: "Bayern München", Gender: "men"},
		{ID: "esp-atletico-madrid", Name: "Atlético Madrid", Gender: "men"},
	} {
		if err := s.UpsertTeam(ctx, team); err != nil {
			t.Fatalf("UpsertTeam: %v", err)
		}
	}

	for query, want := range map[string]string{
		"munchen":  "Bayern München",
		"München":  "Bayern München",
		"atletico": "Atlético Madrid",
		"ATLETICO": "Atlético Madrid",
	} {
		got, err := s.FindTeamsByName(ctx, query)
		if err != nil {
			t.Fatalf("FindTeamsByName(%q): %v", query, err)
		}
		if len(got) != 1 || got[0].Name != want {
			t.Errorf("FindTeamsByName(%q) = %+v, want %q", query, got, want)
		}
	}
}

// A database created before the search columns existed must become
// searchable on the next open, not only after the user happens to
// refresh — a silently unsearchable name is a hard bug to notice, since
// the query succeeds and simply returns nothing.
func TestFoldedNamesAreBackfilledForPreexistingRows(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.UpsertPlayer(ctx, models.Player{ID: "p1", Name: "Martín Zubimendi", Gender: "men"}); err != nil {
		t.Fatalf("UpsertPlayer: %v", err)
	}
	// Simulate a row written before the column existed.
	if _, err := s.db.ExecContext(ctx, `UPDATE player SET name_folded = NULL WHERE id = 'p1'`); err != nil {
		t.Fatalf("clearing name_folded: %v", err)
	}
	if got, _ := s.FindPlayersByName(ctx, "martin"); len(got) != 0 {
		t.Fatal("setup: the row should be unsearchable before backfill")
	}

	if err := backfillFoldedNames(s.db); err != nil {
		t.Fatalf("backfillFoldedNames: %v", err)
	}

	got, err := s.FindPlayersByName(ctx, "martin")
	if err != nil {
		t.Fatalf("FindPlayersByName: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("backfill did not make the row searchable, got %d results", len(got))
	}
}

func TestBackfillIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.UpsertPlayer(ctx, models.Player{ID: "p1", Name: "Martín Zubimendi", Gender: "men"}); err != nil {
		t.Fatalf("UpsertPlayer: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := backfillFoldedNames(s.db); err != nil {
			t.Fatalf("backfill pass %d: %v", i, err)
		}
	}
	if got, _ := s.FindPlayersByName(ctx, "martin"); len(got) != 1 {
		t.Errorf("repeated backfills changed the result: %d", len(got))
	}
}
