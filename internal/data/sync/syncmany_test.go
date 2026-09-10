package sync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

// flakySource fails for specific competition/season pairs, so partial
// failure can be asserted without a network.
type flakySource struct {
	fakeSource
	failOn map[string]bool
	calls  int
}

func (f *flakySource) key(comp provider.CompetitionRef, season provider.SeasonRef) string {
	return comp.CompetitionID + "/" + season.Label
}

func (f *flakySource) Teams(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	f.calls++
	if f.failOn[f.key(comp, season)] {
		return nil, errors.New("upstream exploded")
	}
	return []models.Team{{
		ID:      "team-" + comp.CompetitionID,
		Name:    comp.Name + " FC",
		Country: "Testland", Gender: "men",
	}}, nil
}

func (f *flakySource) Standings(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	pos := 1
	return []models.TeamSeasonStats{{
		TeamID: "team-" + comp.CompetitionID, CompetitionID: comp.CompetitionID, SeasonID: season.SeasonID,
		Position: &pos, MatchesPlayed: 38, Points: 90, Source: "fake", UpdatedAt: time.Now(),
	}}, nil
}

func (f *flakySource) Players(_ context.Context, _ provider.CompetitionRef, _ provider.SeasonRef) ([]models.Player, error) {
	return nil, nil
}

func (f *flakySource) AllRosterStats(_ context.Context, _ provider.CompetitionRef, _ provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	return nil, nil
}

func newSyncer(t *testing.T, src sync.Source) *sync.Syncer {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return sync.New(src, sqlite.NewStore(db))
}

func competitions(ids ...string) []models.Competition {
	comps := make([]models.Competition, 0, len(ids))
	for _, id := range ids {
		comps = append(comps, models.Competition{
			ID: id, Name: id, Country: "Testland", Tier: 1, Gender: "men",
		})
	}
	return comps
}

func TestSyncManyCoversEveryCompetitionSeasonPair(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{}}
	syncer := newSyncer(t, src)

	results := syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league"), []string{"2025", "2024"}, nil)

	if len(results) != 4 {
		t.Fatalf("got %d results, want 2 competitions x 2 seasons", len(results))
	}
	summary := sync.Summarize(results)
	if summary.Failed != 0 || summary.Succeeded != 4 {
		t.Errorf("unexpected summary: %+v", summary)
	}
}

// The point of SyncMany: one bad competition-season must not cost the run.
func TestSyncManyIsolatesAFailingCompetition(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{"b-league/2025": true}}
	syncer := newSyncer(t, src)

	results := syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league", "c-league"), []string{"2025"}, nil)

	summary := sync.Summarize(results)
	if summary.Failed != 1 {
		t.Errorf("expected exactly 1 failure, got %+v", summary)
	}
	if summary.Succeeded != 2 {
		t.Errorf("the other competitions should still have synced, got %+v", summary)
	}

	// And the failure must be attributable, not just counted.
	var failed []string
	for _, r := range results {
		if r.Failed() {
			failed = append(failed, r.Competition.ID)
		}
	}
	if len(failed) != 1 || failed[0] != "b-league" {
		t.Errorf("failed units = %v, want just b-league", failed)
	}
}

func TestSyncManyContinuesToLaterSeasonsAfterOneFails(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{"a-league/2025": true}}
	syncer := newSyncer(t, src)

	results := syncer.SyncMany(context.Background(),
		competitions("a-league"), []string{"2025", "2024", "2023"}, nil)

	summary := sync.Summarize(results)
	if summary.Succeeded != 2 {
		t.Errorf("later seasons should still sync after an earlier one fails: %+v", summary)
	}
}

// A cancelled context is the user interrupting, which should stop the run.
func TestSyncManyStopsOnContextCancellation(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{}}
	syncer := newSyncer(t, src)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := syncer.SyncMany(ctx, competitions("a-league", "b-league"), []string{"2025", "2024"}, nil)

	if len(results) != 1 {
		t.Fatalf("cancelled run produced %d results, want it to stop after the first check", len(results))
	}
	if !errors.Is(results[0].Err, context.Canceled) {
		t.Errorf("expected a context.Canceled result, got %v", results[0].Err)
	}
	if src.calls != 0 {
		t.Errorf("cancelled run still made %d upstream calls", src.calls)
	}
}

func TestSummarizeTotalsOnlySuccessfulUnits(t *testing.T) {
	results := []sync.UnitResult{
		{Result: sync.Result{Teams: 20, TeamStats: 20, Players: 500, PlayerStats: 500}},
		{Err: errors.New("boom"), Result: sync.Result{Teams: 99}},
	}

	s := sync.Summarize(results)
	if s.Totals.Teams != 20 {
		t.Errorf("a failed unit's partial counts should not be totalled: %+v", s.Totals)
	}
	if s.Succeeded != 1 || s.Failed != 1 {
		t.Errorf("unexpected counts: %+v", s)
	}
}

func TestSyncManyReportsProgressAsUnitsFinish(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{"b-league/2025": true}}
	syncer := newSyncer(t, src)

	var seen []string
	results := syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league"), []string{"2025"},
		func(u sync.UnitResult) {
			status := "ok"
			if u.Failed() {
				status = "fail"
			}
			seen = append(seen, u.Competition.ID+":"+status)
		})

	if len(seen) != len(results) {
		t.Fatalf("progress fired %d times for %d results", len(seen), len(results))
	}
	want := []string{"a-league:ok", "b-league:fail"}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("progress %d = %q, want %q", i, seen[i], want[i])
		}
	}
}

// A run against a dead host would otherwise pay the full network timeout
// for every competition-season in turn. Consecutive failures are the
// signal to stop.
func TestSyncManyGivesUpAfterConsecutiveFailures(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{}}
	for _, id := range []string{"a-league", "b-league", "c-league", "d-league", "e-league"} {
		src.failOn[id+"/2025"] = true
	}
	syncer := newSyncer(t, src)
	syncer.MaxConsecutiveFailures = 3

	results := syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league", "c-league", "d-league", "e-league"),
		[]string{"2025"}, nil)

	s := sync.Summarize(results)
	if s.Failed != 3 {
		t.Errorf("attempted %d units, want it to stop after 3", s.Failed)
	}
	if s.Skipped != 2 {
		t.Errorf("skipped %d units, want the remaining 2", s.Skipped)
	}
	// Only three units should have reached the source.
	if src.calls != 3 {
		t.Errorf("made %d upstream calls after giving up, want 3", src.calls)
	}
}

// A run where every other competition works is healthy and must not be
// cut short — total failures are not the signal, consecutive ones are.
func TestInterleavedFailuresDoNotTripTheCircuitBreaker(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{
		"a-league/2025": true,
		"c-league/2025": true,
		"e-league/2025": true,
	}}
	syncer := newSyncer(t, src)
	syncer.MaxConsecutiveFailures = 3

	results := syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league", "c-league", "d-league", "e-league"),
		[]string{"2025"}, nil)

	s := sync.Summarize(results)
	if s.Skipped != 0 {
		t.Errorf("skipped %d units; three non-consecutive failures should not stop a run", s.Skipped)
	}
	if s.Succeeded != 2 || s.Failed != 3 {
		t.Errorf("unexpected summary: %+v", s)
	}
}

func TestSkippedUnitsAreDistinctFromFailures(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{
		"a-league/2025": true, "b-league/2025": true,
	}}
	syncer := newSyncer(t, src)
	syncer.MaxConsecutiveFailures = 2

	results := syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league", "c-league"), []string{"2025"}, nil)

	last := results[len(results)-1]
	if !last.Skipped {
		t.Fatal("the unit after the breaker tripped should be marked skipped")
	}
	if last.Err == nil {
		t.Error("a skipped unit should still explain why it was skipped")
	}
}

func TestSyncRecordsEveryAttemptInTheSyncLog(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{"b-league/2025": true}}
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)
	syncer := sync.New(src, store).WithProviderName("fake")

	syncer.SyncMany(context.Background(),
		competitions("a-league", "b-league"), []string{"2025"}, nil)

	ok, err := store.LatestSyncRun(context.Background(), "a-league", "a-league:2025")
	if err != nil {
		t.Fatalf("LatestSyncRun (success): %v", err)
	}
	if !ok.Succeeded() {
		t.Errorf("successful sync recorded as %q", ok.Status)
	}
	if ok.Provider != "fake" {
		t.Errorf("provider = %q, want the name the syncer was given", ok.Provider)
	}

	// A failure must be recorded too: "we tried an hour ago and it broke"
	// is different from "we haven't tried since Tuesday", and a staleness
	// indicator that hid failures would show the wrong one.
	failed, err := store.LatestSyncRun(context.Background(), "b-league", "b-league:2025")
	if err != nil {
		t.Fatalf("LatestSyncRun (failure): %v", err)
	}
	if failed.Succeeded() {
		t.Error("failed sync recorded as a success")
	}
	if failed.Error == "" {
		t.Error("failed sync recorded without its error message")
	}
}

// A logging failure must not turn a successful refresh into an error.
func TestSyncSucceedsEvenIfTheSyncLogCannotBeWritten(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{}}
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE sync_log`); err != nil {
		t.Fatalf("drop sync_log: %v", err)
	}

	syncer := sync.New(src, sqlite.NewStore(db))
	results := syncer.SyncMany(context.Background(), competitions("a-league"), []string{"2025"}, nil)

	if sync.Summarize(results).Succeeded != 1 {
		t.Errorf("a broken audit table should not fail the refresh: %+v", results[0].Err)
	}
}

// A failed refresh must leave nothing behind. Writing the competition and
// season before fetching would create a season that exists with no data in
// it, which `footy status` and the League screen would then report as
// cached.
func TestAFailedSyncWritesNothing(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{"a-league/2025": true}}
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)
	syncer := sync.New(src, store)

	results := syncer.SyncMany(context.Background(), competitions("a-league"), []string{"2025"}, nil)
	if !results[0].Failed() {
		t.Fatal("expected the unit to fail")
	}

	summary, err := store.CacheSummary(context.Background())
	if err != nil {
		t.Fatalf("CacheSummary: %v", err)
	}
	if len(summary) != 0 {
		t.Errorf("a failed sync left %d competition(s) behind: %+v", len(summary), summary)
	}

	// The attempt itself is still recorded — that's the whole point of
	// the sync log.
	if _, err := store.LatestSyncRun(context.Background(), "a-league", "a-league:2025"); err != nil {
		t.Errorf("the failed attempt should still be logged: %v", err)
	}
}

func TestASuccessfulSyncIsVisibleInTheCacheSummary(t *testing.T) {
	src := &flakySource{failOn: map[string]bool{}}
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)
	syncer := sync.New(src, store)

	syncer.SyncMany(context.Background(), competitions("a-league"), []string{"2025", "2024"}, nil)

	summary, err := store.CacheSummary(context.Background())
	if err != nil {
		t.Fatalf("CacheSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("got %d competitions", len(summary))
	}
	got := summary[0]
	if got.Seasons != 2 {
		t.Errorf("seasons = %d, want 2", got.Seasons)
	}
	// Counts must not multiply against each other: one team across two
	// seasons is one team, not two.
	if got.Teams != 1 {
		t.Errorf("teams = %d, want 1 distinct team", got.Teams)
	}
	if got.EarliestSeason != "2024" || got.LatestSeason != "2025" {
		t.Errorf("season range = %s..%s", got.EarliestSeason, got.LatestSeason)
	}
	if got.LastSync == nil || !got.LastSync.Succeeded() {
		t.Errorf("expected a successful last sync, got %+v", got.LastSync)
	}
}
