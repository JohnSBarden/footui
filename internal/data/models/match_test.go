package models

import (
	"testing"
	"time"
)

func ip(n int) *int         { return &n }
func fp(f float64) *float64 { return &f }
func day(d int) time.Time   { return time.Date(2026, 3, d, 19, 0, 0, 0, time.UTC) }
func stats(poss float64, shots int) *MatchTeamStats {
	return &MatchTeamStats{Possession: fp(poss), Shots: ip(shots)}
}

func TestSummarizeMatchesCountsResultsAndSkipsMissingStats(t *testing.T) {
	const me = "nat-arcadia"
	matches := []Match{
		// Newest first, as storage returns them; the summary mustn't care.
		{ID: "4", Kickoff: day(9), Status: MatchScheduled, HomeTeamID: me, AwayTeamID: "nat-x"},
		{ID: "3", Kickoff: day(7), Status: MatchPens, HomeTeamID: "nat-x", AwayTeamID: me,
			HomeScore: ip(1), AwayScore: ip(1), HomePens: ip(4), AwayPens: ip(5)},
		{ID: "2", Kickoff: day(5), Status: MatchFullTime, HomeTeamID: me, AwayTeamID: "nat-y",
			HomeScore: ip(0), AwayScore: ip(2), HomeStats: &MatchTeamStats{Shots: ip(10)}},
		{ID: "1", Kickoff: day(3), Status: MatchAET, HomeTeamID: "nat-z", AwayTeamID: me,
			HomeScore: ip(1), AwayScore: ip(3), AwayStats: stats(60, 20), HomeStats: stats(40, 2)},
		{ID: "0", Kickoff: day(1), Status: MatchCancelled, HomeTeamID: me, AwayTeamID: "nat-q"},
	}
	s := SummarizeMatches(me, matches)

	if s.Played != 3 || s.Won != 1 || s.Drawn != 1 || s.Lost != 1 {
		t.Errorf("record %+v: a shoot-out is a draw, aet counts by its score, unplayed matches don't count", s)
	}
	if s.GoalsFor != 4 || s.GoalsAgainst != 4 {
		t.Errorf("goals %d-%d", s.GoalsFor, s.GoalsAgainst)
	}
	if s.Form != "WLD" {
		t.Errorf("form %q, want oldest first", s.Form)
	}
	// Possession is known for one match only: its average is that one, not
	// diluted by matches that didn't report it.
	if s.Possession == nil || *s.Possession != 60 {
		t.Errorf("possession %v", s.Possession)
	}
	if s.Shots == nil || *s.Shots != 15 {
		t.Errorf("shots %v, want (20+10)/2", s.Shots)
	}
	if s.Corners != nil {
		t.Error("no match reported corners, so there is no average")
	}
}

func TestFormKeepsTheLastFive(t *testing.T) {
	var ms []Match
	for d := 1; d <= 7; d++ {
		ms = append(ms, Match{Kickoff: day(d), Status: MatchFullTime, HomeTeamID: "a", AwayTeamID: "b",
			HomeScore: ip(d % 2), AwayScore: ip(0)})
	}
	if got := SummarizeMatches("a", ms).Form; got != "WDWDW" {
		t.Errorf("form %q", got)
	}
}

func TestFilterPresets(t *testing.T) {
	wc := Competition{ID: "world-cup", Category: CategoryTournament}
	wcq := Competition{ID: "wcq-uefa", Category: CategoryQualifier}
	unl := Competition{ID: "nations-league", Category: CategoryNationsLeague}
	fr := Competition{ID: "int-friendly", Category: CategoryFriendly}
	for _, tc := range []struct {
		f    MatchFilter
		want [4]bool
	}{
		{MatchFilter{}, [4]bool{true, true, true, true}},
		{MatchFilter{Preset: PresetAll}, [4]bool{true, true, true, true}},
		{MatchFilter{Preset: PresetCompetitive}, [4]bool{true, true, true, false}},
		{MatchFilter{Preset: PresetTournaments}, [4]bool{true, false, false, false}},
		{MatchFilter{Competitions: []string{"int-friendly", "wcq-uefa"}}, [4]bool{false, true, false, true}},
		{MatchFilter{Competitions: []string{}}, [4]bool{false, false, false, false}},
	} {
		for i, c := range []Competition{wc, wcq, unl, fr} {
			if got := tc.f.Keeps(c); got != tc.want[i] {
				t.Errorf("%+v keeps %s = %v", tc.f, c.ID, got)
			}
		}
	}
}

func TestCupsAreClubCompetitionsNotInternationalOnes(t *testing.T) {
	cup := Competition{ID: "eng-fa-cup", Category: CategoryCup}
	if cup.International() || cup.HasTable() || !cup.Cup() {
		t.Errorf("a domestic cup: international %v, table %v, cup %v", cup.International(), cup.HasTable(), cup.Cup())
	}
	wc := Competition{ID: "world-cup", Category: CategoryTournament}
	if !wc.International() || wc.Cup() {
		t.Error("a tournament is international, and not a domestic cup")
	}
	if (Competition{ID: "eng-premier-league"}).Cup() {
		t.Error("a league is not a cup")
	}
}
