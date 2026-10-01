package understat

import (
	"encoding/json"
	"os"
	"testing"
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

func TestTeamIDIsStableAndURLSafe(t *testing.T) {
	got := teamID("eng-premier-league", "Manchester City")
	want := "understat:eng-manchester-city"
	if got != want {
		t.Errorf("teamID() = %q, want %q", got, want)
	}
}
