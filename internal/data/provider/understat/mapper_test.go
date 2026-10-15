package understat

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

func loadFixture(t *testing.T) *leagueData {
	t.Helper()
	raw, err := os.ReadFile("testdata/sample_league_data.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var data leagueData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return &data
}

func TestMapStandingsAggregatesAndRanks(t *testing.T) {
	data := loadFixture(t)
	stats := mapStandings(data, "eng-premier-league", "eng-premier-league:2025")

	if len(stats) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(stats))
	}

	byTeam := map[string]int{}
	for i, s := range stats {
		byTeam[s.TeamID] = i
		if s.Source != "understat" {
			t.Errorf("expected source 'understat', got %q", s.Source)
		}
	}

	arsenal := stats[byTeam["understat:eng-arsenal"]]
	if arsenal.Points != 4 || arsenal.MatchesPlayed != 2 || arsenal.GoalsFor != 3 || arsenal.GoalsAgainst != 1 {
		t.Errorf("unexpected Arsenal aggregate: %+v", arsenal)
	}
	if arsenal.XG == nil || *arsenal.XG != 3.1 {
		t.Errorf("expected Arsenal xG 3.1, got %v", arsenal.XG)
	}

	// Arsenal (4 pts) should rank above Chelsea (3 pts).
	if *arsenal.Position != 1 {
		t.Errorf("expected Arsenal ranked 1st, got %v", *arsenal.Position)
	}
}

func TestMapPlayersAttributesTransferredPlayerToMostRecentClub(t *testing.T) {
	data := loadFixture(t)
	players := mapPlayers(data, "eng-premier-league", "eng-premier-league:2025")

	var transfer *string
	for _, p := range players {
		if p.PlayerID == "understat:101" {
			transfer = &p.TeamID
		}
	}
	if transfer == nil {
		t.Fatal("expected player understat-101 in mapped output")
	}
	// team_title "Chelsea,Arsenal" -> most recent club is Arsenal, not a
	// bogus concatenated "Chelsea,Arsenal" slug with no matching team row.
	if *transfer != "understat:eng-arsenal" {
		t.Errorf("expected transferred player attributed to eng-arsenal, got %q", *transfer)
	}
}

func TestMapPlayerEntitiesExtractsIdentity(t *testing.T) {
	data := loadFixture(t)
	players := mapPlayerEntities(data)

	// One identity row per player entry in the payload, whatever the
	// fixture happens to contain.
	if len(players) != len(data.Players) {
		t.Fatalf("expected %d players, got %d", len(data.Players), len(players))
	}
	for _, p := range players {
		if p.Gender != "men" {
			t.Errorf("expected gender 'men', got %q", p.Gender)
		}
		if p.Name == "" {
			t.Errorf("expected non-empty player name for id %q", p.ID)
		}
	}
}

func TestMapMatchesReadsResultsFixturesAndXG(t *testing.T) {
	data := loadFixture(t)
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	matches := mapMatches(data, "eng-premier-league", "eng-premier-league:2025", updated)

	// The third entry names a club the payload has no team for, so it
	// can't be stored against anyone.
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2: %+v", len(matches), matches)
	}
	m := matches[0]
	if m.ID != "understat:900101" || m.HomeTeamID != "understat:eng-arsenal" || m.AwayTeamID != "understat:eng-chelsea" {
		t.Errorf("identity %+v", m)
	}
	if m.Status != models.MatchFullTime || *m.HomeScore != 2 || *m.AwayScore != 0 {
		t.Errorf("result %+v", m)
	}
	if !m.Kickoff.Equal(time.Date(2025, 8, 16, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("kickoff %v: understat's times are UTC", m.Kickoff)
	}
	if m.HomeStats == nil || *m.HomeStats.XG != 2.1043 || *m.AwayStats.XG != 0.81 {
		t.Errorf("xG %+v / %+v", m.HomeStats, m.AwayStats)
	}
	if m.CompetitionID != "eng-premier-league" || m.SeasonID != "eng-premier-league:2025" || m.Source != "understat" || !m.UpdatedAt.Equal(updated) {
		t.Errorf("provenance %+v", m)
	}

	fixture := matches[1]
	if fixture.Status != models.MatchScheduled || fixture.HomeScore != nil || fixture.HomeStats != nil {
		t.Errorf("an unplayed fixture has no score or stats: %+v", fixture)
	}
}

func TestTeamIDIsStableAndURLSafe(t *testing.T) {
	got := teamID("eng-premier-league", "Manchester City")
	want := "understat:eng-manchester-city"
	if got != want {
		t.Errorf("teamID() = %q, want %q", got, want)
	}
}
