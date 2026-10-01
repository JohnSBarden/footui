package understat

import (
	"math"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
)

func findTeam(t *testing.T, stats []models.TeamSeasonStats, teamID string) models.TeamSeasonStats {
	t.Helper()
	for _, s := range stats {
		if s.TeamID == teamID {
			return s
		}
	}
	t.Fatalf("team %q not in mapped standings", teamID)
	return models.TeamSeasonStats{}
}

func closeTo(got, want float64) bool {
	return math.Abs(got-want) < 1e-6
}

// PPDA is a ratio, so a season figure must be the ratio of summed
// numerators and denominators. Arsenal's two matches are 200/10 (=20) and
// 100/20 (=5): averaging the per-match ratios gives 12.5, while the
// correct season PPDA is 300/30 = 10. The fixture is built so those two
// answers differ.
func TestSeasonPPDASumsBeforeDividing(t *testing.T) {
	stats := mapStandings(loadFixture(t), "eng-premier-league", "eng-premier-league:2025")
	arsenal := findTeam(t, stats, "understat:eng-arsenal")

	got, ok := arsenal.Extras[models.StatPPDA]
	if !ok {
		t.Fatal("PPDA missing from extras")
	}
	if !closeTo(got, 10.0) {
		t.Errorf("season PPDA = %v, want 10 (300 att / 30 def); 12.5 would mean per-match ratios were averaged", got)
	}

	allowed, ok := arsenal.Extras[models.StatPPDAAllowed]
	if !ok {
		t.Fatal("PPDA allowed missing from extras")
	}
	if !closeTo(allowed, 4.0) {
		t.Errorf("season PPDA allowed = %v, want 4 (200 att / 50 def)", allowed)
	}
}

func TestAdvancedTeamStatsAreSummedAcrossMatches(t *testing.T) {
	stats := mapStandings(loadFixture(t), "eng-premier-league", "eng-premier-league:2025")
	arsenal := findTeam(t, stats, "understat:eng-arsenal")

	cases := map[string]float64{
		models.StatNPxG:           2.9,  // 1.9 + 1.0
		models.StatNPxGA:          1.7,  // 0.5 + 1.2
		models.StatExpectedPoints: 3.5,  // 2.4 + 1.1
		models.StatDeep:           20.0, // 12 + 8
		models.StatDeepAllowed:    10.0, // 3 + 7
	}
	for key, want := range cases {
		got, ok := arsenal.Extras[key]
		if !ok {
			t.Errorf("extras missing %q", key)
			continue
		}
		if !closeTo(got, want) {
			t.Errorf("extras[%q] = %v, want %v", key, got, want)
		}
	}
}

// A team with no matches played has no meaningful advanced stats, and
// emitting zeroes would read as "pressed nobody" rather than "no data".
func TestTeamWithNoMatchesHasNoExtras(t *testing.T) {
	data := &leagueData{Teams: map[string]teamEntry{
		"9": {ID: "9", Title: "Newly Promoted", History: nil},
	}}

	stats := mapStandings(data, "eng-premier-league", "eng-premier-league:2025")
	if len(stats) != 1 {
		t.Fatalf("got %d rows", len(stats))
	}
	if stats[0].Extras != nil {
		t.Errorf("expected no extras for a team with no matches, got %v", stats[0].Extras)
	}
}

// A division by zero in the PPDA ratio must produce an absent key, not NaN.
func TestZeroDefensiveActionsOmitsPPDARatherThanEmittingNaN(t *testing.T) {
	data := &leagueData{Teams: map[string]teamEntry{
		"9": {ID: "9", Title: "Passive", History: []matchEntry{
			{PPDA: ppdaEntry{Att: 120, Def: 0}, PPDAAllowed: ppdaEntry{Att: 100, Def: 0}},
		}},
	}}

	stats := mapStandings(data, "eng-premier-league", "eng-premier-league:2025")
	if _, ok := stats[0].Extras[models.StatPPDA]; ok {
		t.Error("PPDA should be omitted when there were no defensive actions, not divided by zero")
	}
	// The summable stats are still present.
	if _, ok := stats[0].Extras[models.StatDeep]; !ok {
		t.Error("summable extras should still be present")
	}
}

func TestPlayerAdvancedStatsAreMapped(t *testing.T) {
	players := mapPlayers(loadFixture(t), "eng-premier-league", "eng-premier-league:2025")

	var striker models.PlayerSeasonStats
	for _, p := range players {
		if p.PlayerID == "understat:100" {
			striker = p
		}
	}
	if striker.PlayerID == "" {
		t.Fatal("striker not mapped")
	}

	if striker.Shots == nil || *striker.Shots != 10 {
		t.Errorf("shots = %v, want 10", striker.Shots)
	}
	if striker.KeyPasses == nil || *striker.KeyPasses != 3 {
		t.Errorf("key passes = %v, want 3", striker.KeyPasses)
	}
	if striker.NonPenaltyGoals == nil || *striker.NonPenaltyGoals != 3 {
		t.Errorf("non-penalty goals = %v, want 3", striker.NonPenaltyGoals)
	}
	if striker.NPxG == nil || !closeTo(*striker.NPxG, 2.5) {
		t.Errorf("npxG = %v, want 2.5", striker.NPxG)
	}
}

// A provider omitting an advanced field must yield nil, not zero: "not
// reported" and "took zero shots" are different facts and the TUI renders
// them differently.
func TestMissingPlayerAdvancedStatsAreNilNotZero(t *testing.T) {
	players := mapPlayers(loadFixture(t), "eng-premier-league", "eng-premier-league:2025")

	var sparse models.PlayerSeasonStats
	for _, p := range players {
		if p.PlayerID == "understat:102" {
			sparse = p
		}
	}
	if sparse.PlayerID == "" {
		t.Fatal("sparse player not mapped")
	}

	if sparse.Shots != nil {
		t.Errorf("shots = %v, want nil for an absent field", *sparse.Shots)
	}
	if sparse.KeyPasses != nil {
		t.Errorf("key passes = %v, want nil for an absent field", *sparse.KeyPasses)
	}
	if sparse.NPxG != nil {
		t.Errorf("npxG = %v, want nil for an absent field", *sparse.NPxG)
	}
}
