package sync

import (
	"testing"
	"time"
)

func TestCurrentSeasonStartYearRollsOverInAugust(t *testing.T) {
	cases := []struct {
		when time.Time
		want string
	}{
		// Mid-season: still the season that started last August.
		{time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC), "2025"},
		// May, just after a season ends: still that season.
		{time.Date(2026, time.May, 30, 0, 0, 0, 0, time.UTC), "2025"},
		// July, between seasons: the new season has no matches yet, so the
		// completed one is still what a refresh should fetch.
		{time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC), "2025"},
		// August onwards: the new season.
		{time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC), "2026"},
		{time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC), "2026"},
		{time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC), "2026"},
	}
	for _, c := range cases {
		if got := CurrentSeasonStartYear(c.when); got != c.want {
			t.Errorf("CurrentSeasonStartYear(%s) = %q, want %q", c.when.Format("2006-01-02"), got, c.want)
		}
	}
}

func TestRecentSeasonYearsCountsBackwards(t *testing.T) {
	now := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)

	got := RecentSeasonYears(now, 3)
	want := []string{"2026", "2025", "2024"}
	if len(got) != len(want) {
		t.Fatalf("RecentSeasonYears(_, 3) = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("season %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRecentSeasonYearsTreatsZeroAsOne(t *testing.T) {
	now := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)

	for _, n := range []int{0, -5} {
		got := RecentSeasonYears(now, n)
		if len(got) != 1 || got[0] != "2026" {
			t.Errorf("RecentSeasonYears(_, %d) = %v, want just the current season", n, got)
		}
	}
}

func TestNewSeasonBuildsAStableID(t *testing.T) {
	s := NewSeason("eng-premier-league", "2025")
	if s.ID != "eng-premier-league:2025" {
		t.Errorf("season ID = %q", s.ID)
	}
	if s.CompetitionID != "eng-premier-league" || s.Label != "2025" {
		t.Errorf("unexpected season: %+v", s)
	}
}
