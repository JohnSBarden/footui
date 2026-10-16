package league

import "testing"

func TestZones(t *testing.T) {
	cases := []struct {
		comp      string
		year, pos int
		want      zone
	}{
		{"eng-premier-league", 2026, 1, zoneUCL},
		{"eng-premier-league", 2026, 4, zoneUCL},
		{"eng-premier-league", 2026, 5, zoneUEL},
		{"eng-premier-league", 2026, 18, zoneDown},
		{"eng-premier-league", 2026, 10, zoneNone},
		{"ger-bundesliga", 2026, 17, zoneDown},
		{"ger-bundesliga", 2026, 16, zoneNone},    // the play-off place isn't guessed at
		{"eng-premier-league", 2019, 1, zoneNone}, // before the seeded rule
		{"usa-mls", 2026, 1, zoneNone},            // no rule, no guess
		{"eng-premier-league", 2026, 0, zoneNone}, // no position
	}
	for _, c := range cases {
		if got := zoneFor(c.comp, c.year, c.pos); got != c.want {
			t.Errorf("%s %d #%d = %v, want %v", c.comp, c.year, c.pos, got, c.want)
		}
	}
}

func TestStartYear(t *testing.T) {
	for label, want := range map[string]int{"2024-25": 2024, "2026": 2026, "": 0, "abc": 0} {
		if got := startYear(label); got != want {
			t.Errorf("startYear(%q) = %d, want %d", label, got, want)
		}
	}
}
