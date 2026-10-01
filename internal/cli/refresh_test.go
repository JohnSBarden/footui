package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
)

var testNow = time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)

func TestResolveCompetitionsDefaultsToOne(t *testing.T) {
	f := &refreshFlags{competitionID: "eng-premier-league"}

	comps, err := f.resolveCompetitions()
	if err != nil {
		t.Fatalf("resolveCompetitions: %v", err)
	}
	if len(comps) != 1 || comps[0].ID != "eng-premier-league" {
		t.Errorf("got %+v", comps)
	}
}

func TestResolveCompetitionsAllCoversTheWholeScope(t *testing.T) {
	f := &refreshFlags{all: true}

	comps, err := f.resolveCompetitions()
	if err != nil {
		t.Fatalf("resolveCompetitions: %v", err)
	}
	if len(comps) != len(sync.Scope) {
		t.Errorf("--all returned %d competitions, want the full scope of %d", len(comps), len(sync.Scope))
	}
}

func TestUnknownCompetitionListsTheSupportedOnes(t *testing.T) {
	f := &refreshFlags{competitionID: "ger-frauen-bundesliga"}

	_, err := f.resolveCompetitions()
	if err == nil {
		t.Fatal("expected an error for an out-of-scope competition")
	}
	// The message has to tell the user what they can ask for.
	if !strings.Contains(err.Error(), "eng-premier-league") {
		t.Errorf("error should list supported competitions, got: %v", err)
	}
}

func planFor(t *testing.T, f *refreshFlags, ids ...string) map[string][]string {
	t.Helper()
	var comps []models.Competition
	for _, id := range ids {
		c, ok := sync.FindCompetition(id)
		if !ok {
			t.Fatalf("%s not in scope", id)
		}
		comps = append(comps, c)
	}
	out := map[string][]string{}
	for _, j := range f.plan(comps, testNow) {
		out[j.Competition.ID] = j.Seasons
	}
	return out
}

func TestPlanDefaultsToEachCompetitionsCurrentSeason(t *testing.T) {
	got := planFor(t, &refreshFlags{years: 1}, "eng-premier-league", "usa-mls")
	if s := got["eng-premier-league"]; len(s) != 1 || s[0] != "2026" {
		t.Errorf("premier league %v, want 2026-27", s)
	}
	if s := got["usa-mls"]; len(s) != 1 || s[0] != "2026" {
		t.Errorf("MLS %v, want 2026", s)
	}
}

func TestPlanCountsBackWithYearsButNotPastTheProvidersHistory(t *testing.T) {
	got := planFor(t, &refreshFlags{years: 10}, "eng-premier-league", "esp-liga-f")
	if s := got["eng-premier-league"]; len(s) != 10 || s[0] != "2026" || s[9] != "2017" {
		t.Errorf("ten Premier League seasons = %v", s)
	}
	// Liga F's table only goes back to 2022-23.
	if s := got["esp-liga-f"]; len(s) != 5 || s[len(s)-1] != "2022" {
		t.Errorf("Liga F = %v, want 2026 back to 2022 only", s)
	}
}

// Asking for a specific season and "the N most recent" at once has no
// coherent meaning, so the explicit season wins.
func TestExplicitSeasonOverridesYears(t *testing.T) {
	got := planFor(t, &refreshFlags{season: "2019", years: 5}, "eng-premier-league")
	if s := got["eng-premier-league"]; len(s) != 1 || s[0] != "2019" {
		t.Errorf("plan = %v, want only the explicitly requested season", s)
	}
}

func TestDryRunPlanShowsEachCompetitionsSeasonLabels(t *testing.T) {
	f := &refreshFlags{years: 2}
	comps := []models.Competition{}
	for _, id := range []string{"eng-premier-league", "usa-mls"} {
		c, _ := sync.FindCompetition(id)
		comps = append(comps, c)
	}

	var buf bytes.Buffer
	printPlan(&buf, f.plan(comps, testNow), "/tmp/footy.db")
	out := buf.String()

	if !strings.Contains(out, "2026-27, 2025-26") {
		t.Errorf("split-year seasons should show as the TUI does\n---\n%s", out)
	}
	if !strings.Contains(out, "2026, 2025") {
		t.Errorf("MLS seasons are calendar years\n---\n%s", out)
	}
	if !strings.Contains(out, "nothing will be fetched") {
		t.Errorf("a dry run must say it isn't doing anything\n---\n%s", out)
	}
	if !strings.Contains(out, "4 competition-season") {
		t.Errorf("plan should state the unit count (2 competitions x 2 seasons)\n---\n%s", out)
	}
}

func TestReportFailsOnlyWhenEverythingFailed(t *testing.T) {
	comp := sync.Scope[0]
	season := sync.NewSeason(comp.ID, "2025")

	allFailed := []sync.UnitResult{{Competition: comp, Season: season, Err: errBoom{}}}
	if err := report(&bytes.Buffer{}, allFailed, "/tmp/db"); err == nil {
		t.Error("a run where nothing succeeded should report an error")
	}

	// A partial success still left the cache better off than before;
	// exiting non-zero would misreport that to a cron job.
	partial := []sync.UnitResult{
		{Competition: comp, Season: season, Result: sync.Result{Teams: 20}},
		{Competition: comp, Season: sync.NewSeason(comp.ID, "2024"), Err: errBoom{}},
	}
	var buf bytes.Buffer
	if err := report(&buf, partial, "/tmp/db"); err != nil {
		t.Errorf("a partial success should not be an error, got %v", err)
	}
	if !strings.Contains(buf.String(), "1 competition-season(s) failed") {
		t.Errorf("partial failures must still be reported\n---\n%s", buf.String())
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
