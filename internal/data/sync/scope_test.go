package sync

import (
	"testing"
	"time"
)

func cov(t *testing.T, id string) Coverage {
	t.Helper()
	c, ok := FindCoverage(id)
	if !ok {
		t.Fatalf("%s not covered", id)
	}
	return c
}

func TestCalendarYearLeaguesRollOverInMarch(t *testing.T) {
	mls, pl := cov(t, "usa-mls"), cov(t, "eng-premier-league")
	for _, tc := range []struct {
		when    time.Time
		mls, pl string
	}{
		// February: MLS hasn't kicked off, the Premier League is mid-season.
		{time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), "2025", "2025"},
		// April: the new MLS season is on; the European one is 2025-26.
		{time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), "2026", "2025"},
		// September: both new.
		{time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), "2026", "2026"},
	} {
		if got := mls.CurrentSeason(tc.when); got != tc.mls {
			t.Errorf("%s: MLS current %s, want %s", tc.when.Format("Jan"), got, tc.mls)
		}
		if got := pl.CurrentSeason(tc.when); got != tc.pl {
			t.Errorf("%s: Premier League current %s, want %s", tc.when.Format("Jan"), got, tc.pl)
		}
	}
}

func TestAvailableSeasonsStopAtTheProvidersFirstSeason(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	got := cov(t, "eur-conference-league").AvailableSeasons(now)
	if len(got) != 6 || got[0] != "2026" || got[5] != "2021" {
		t.Errorf("Conference League seasons %v, want 2026 back to its first, 2021", got)
	}
	if got := cov(t, "eng-premier-league").RecentSeasons(now, 100); got[len(got)-1] != "2014" {
		t.Errorf("Premier League history should stop at understat's 2014, got %v", got)
	}
}

func TestCoverageIDsAreUniqueAndScopeMatches(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Coverages {
		if seen[c.Competition.ID] {
			t.Errorf("%s listed twice", c.Competition.ID)
		}
		seen[c.Competition.ID] = true
		if c.Provider != ProviderUnderstat && c.Provider != ProviderESPN {
			t.Errorf("%s has unknown provider %q", c.Competition.ID, c.Provider)
		}
	}
	if len(Scope) != len(Coverages) {
		t.Errorf("Scope has %d entries, Coverages %d", len(Scope), len(Coverages))
	}
}
