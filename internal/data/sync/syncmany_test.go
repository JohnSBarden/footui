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
