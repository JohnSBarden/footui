package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
	f := &refreshFlags{competitionID: "eng-championship"}

	_, err := f.resolveCompetitions()
	if err == nil {
		t.Fatal("expected an error for an out-of-scope competition")
	}
	// The message has to tell the user what they can ask for; understat's
	// coverage gap is real and this is where a user hits it.
	if !strings.Contains(err.Error(), "eng-premier-league") {
		t.Errorf("error should list supported competitions, got: %v", err)
	}
}

func TestResolveSeasonsDefaultsToTheCurrentSeason(t *testing.T) {
	f := &refreshFlags{years: 1}

	got := f.resolveSeasons(testNow)
	if len(got) != 1 || got[0] != "2026" {
		t.Errorf("resolveSeasons = %v, want just the 2026-27 season", got)
	}
}

func TestResolveSeasonsCountsBackWithYears(t *testing.T) {
	f := &refreshFlags{years: 10}

	got := f.resolveSeasons(testNow)
	if len(got) != 10 {
		t.Fatalf("resolveSeasons returned %d seasons, want 10", len(got))
	}
	if got[0] != "2026" || got[9] != "2017" {
		t.Errorf("ten seasons back = %v", got)
	}
}

// Asking for a specific season and "the N most recent" at once has no
// coherent meaning, so the explicit season wins.
func TestExplicitSeasonOverridesYears(t *testing.T) {
	f := &refreshFlags{season: "2019", years: 5}

	got := f.resolveSeasons(testNow)
	if len(got) != 1 || got[0] != "2019" {
		t.Errorf("resolveSeasons = %v, want only the explicitly requested season", got)
	}
}

func TestDryRunPlanShowsDisplaySeasonLabels(t *testing.T) {
	f := &refreshFlags{all: true, years: 2}
	comps, _ := f.resolveCompetitions()

	var buf bytes.Buffer
	printPlan(&buf, comps, f.resolveSeasons(testNow), "/tmp/footy.db")
	out := buf.String()

	if !strings.Contains(out, "2026-27") || !strings.Contains(out, "2025-26") {
		t.Errorf("plan should show seasons the way the TUI does\n---\n%s", out)
	}
	if !strings.Contains(out, "nothing will be fetched") {
		t.Errorf("a dry run must say it isn't doing anything\n---\n%s", out)
	}
	if !strings.Contains(out, "12 competition-season") {
		t.Errorf("plan should state the unit count (6 competitions x 2 seasons)\n---\n%s", out)
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
