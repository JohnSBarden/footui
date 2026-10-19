package sync

import (
	"strings"
	"testing"
	"time"
)

// Short names sit beside match dates on cards ("10-06 FRND Canada"), so
// none may read as a weekday.
func TestNoShortNameReadsAsAWeekday(t *testing.T) {
	days := map[string]bool{"MON": true, "TUE": true, "WED": true, "THU": true, "FRI": true, "SAT": true, "SUN": true}
	for _, c := range Coverages {
		if days[strings.ToUpper(c.Competition.Abbrev())] {
			t.Errorf("%s abbreviates to %q, which reads as a day", c.Competition.ID, c.Competition.Abbrev())
		}
	}
}

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
	clubs := 0
	for _, c := range Coverages {
		if !c.Competition.International() && !c.Competition.Cup() {
			clubs++
		}
	}
	if len(Scope) != clubs {
		t.Errorf("Scope has %d entries, want the %d club leagues", len(Scope), clubs)
	}
}

func TestBaseIsTheStarterSet(t *testing.T) {
	want := []string{
		"eng-premier-league", "esp-la-liga", "ger-bundesliga", "ita-serie-a", "fra-ligue-1",
		"usa-mls", "eng-wsl", "usa-nwsl", "eur-champions-league", "eur-europa-league", "world-cup",
	}
	var got []string
	for _, c := range Base {
		got = append(got, c.ID)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Base is %v, want %v", got, want)
	}
	if InBase("rus-premier-league") {
		t.Error("the Russian Premier League should be left out of the starter set")
	}
	if _, ok := FindCompetition("rus-premier-league"); !ok {
		t.Error("the Russian Premier League should still be in the catalog")
	}
}

func TestInternationalCoverageCountsEditionsAndYears(t *testing.T) {
	oct2026 := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	wc := cov(t, "world-cup")
	if got := wc.AvailableSeasons(oct2026); strings.Join(got, " ") != "2026 2022 2018 2014 2010" {
		t.Errorf("world cup editions %v", got)
	}
	if wc.CurrentSeason(oct2026) != "2026" || wc.CurrentSeason(time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)) != "2022" {
		t.Error("a tournament's current edition is the latest one whose year has come")
	}
	if got := wc.RecentSeasons(oct2026, 2); strings.Join(got, " ") != "2026 2022" {
		t.Errorf("recent editions %v", got)
	}
	unl := cov(t, "nations-league")
	if unl.CurrentSeason(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)) != "2026" {
		t.Error("a nations league's matches are counted by calendar year, with no rollover month")
	}
	if got := unl.AvailableSeasons(oct2026); got[len(got)-1] != "2018" {
		t.Errorf("nations league years %v, want back to 2018", got)
	}
	if !cov(t, "world-cup").Competition.CalendarYear || cov(t, "nations-league").Competition.CalendarYear {
		t.Error("tournaments show as one year, nations leagues as a split season")
	}
	men, women := InternationalCoverages("men"), InternationalCoverages("women")
	if len(men) != 15 || len(women) != 4 {
		t.Errorf("%d men's and %d women's international competitions", len(men), len(women))
	}
	for _, c := range append(men, women...) {
		if c.Competition.ShortName == "" || !c.Competition.International() {
			t.Errorf("%s needs a short name and a category", c.Competition.ID)
		}
	}
}
