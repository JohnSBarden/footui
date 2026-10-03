package cli

import (
	"context"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

// september is inside the 2026-27 season.
var september = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func seedSeason(t *testing.T, repos storage.Repositories, compID, year string, syncedAt time.Time, status string) {
	t.Helper()
	comp, _ := sync.FindCompetition(compID)
	storagetest.SeedCompetition(t, repos, comp, year, storagetest.SampleStandings())
	if syncedAt.IsZero() {
		return
	}
	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{
		Provider: "understat", CompetitionID: compID, SeasonID: compID + ":" + year,
		StartedAt: syncedAt, FinishedAt: syncedAt, Status: status,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStaleRefreshesPicksFollowedLeaguesOnly(t *testing.T) {
	repos := storagetest.New(t)
	// Followed and stale.
	seedSeason(t, repos, "eng-premier-league", "2026", september.Add(-13*24*time.Hour), models.SyncStatusOK)
	// Followed and fresh.
	seedSeason(t, repos, "esp-la-liga", "2026", september.Add(-time.Hour), models.SyncStatusOK)
	// Followed last season; the new one hasn't been fetched yet.
	seedSeason(t, repos, "ger-bundesliga", "2025", september.Add(-120*24*time.Hour), models.SyncStatusOK)
	// History only: newest season is years old.
	seedSeason(t, repos, "ita-serie-a", "2016", september.Add(-24*time.Hour), models.SyncStatusOK)

	reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour)

	got := map[string]string{}
	for _, r := range reqs {
		if len(r.Seasons) != 1 || r.Seasons[0] != "2026" {
			t.Errorf("%s: seasons %v, want the current season only", r.CompetitionID, r.Seasons)
		}
		got[r.CompetitionID] = r.Why
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want the Premier League and Bundesliga", got)
	}
	if got["eng-premier-league"] != "cached 13 days ago" {
		t.Errorf("premier league reason %q", got["eng-premier-league"])
	}
	if got["ger-bundesliga"] != "new season 2026-27" {
		t.Errorf("bundesliga reason %q", got["ger-bundesliga"])
	}
}

func TestStaleRefreshesBacksOffAfterARecentFailure(t *testing.T) {
	repos := storagetest.New(t)
	seedSeason(t, repos, "eng-premier-league", "2026", september.Add(-2*time.Minute), models.SyncStatusFailed)
	if reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour); len(reqs) != 0 {
		t.Errorf("retried a refresh that failed two minutes ago: %v", reqs)
	}

	seedSeason(t, repos, "esp-la-liga", "2026", september.Add(-time.Hour), models.SyncStatusFailed)
	reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour)
	if len(reqs) != 1 || reqs[0].CompetitionID != "esp-la-liga" {
		t.Errorf("an older failure should be retried, got %v", reqs)
	}
}

func TestStaleRefreshesCanBeDisabled(t *testing.T) {
	repos := storagetest.New(t)
	seedSeason(t, repos, "eng-premier-league", "2026", september.Add(-30*24*time.Hour), models.SyncStatusOK)
	if reqs := staleRefreshes(context.Background(), repos, september, 0); len(reqs) != 0 {
		t.Errorf("--stale-after 0 should disable the check, got %v", reqs)
	}
}

func TestCatalogOffersEverySeasonBackTo2014(t *testing.T) {
	cat := tuiCatalog(september)
	if len(cat) != len(sync.Scope) {
		t.Fatalf("catalog has %d entries, want every covered competition", len(cat))
	}
	seasons := cat[0].Seasons
	if seasons[0] != "2026" || seasons[len(seasons)-1] != "2014" || len(seasons) != 13 {
		t.Errorf("seasons %v, want 2026 down to 2014", seasons)
	}
}

func TestSummarizeUnitsKeepsAPartialDownload(t *testing.T) {
	results := []sync.UnitResult{
		{Season: sync.NewSeason("x", "2025"), Result: sync.Result{TeamStats: 20, PlayerStats: 500}},
		{Season: sync.NewSeason("x", "2024"), Err: context.DeadlineExceeded},
	}
	line, err := summarizeUnits(results)
	if err != nil {
		t.Fatalf("one season succeeding should not be an error: %v", err)
	}
	if want := "1 of 2 seasons — 20 teams, 500 players; failed: 2024-25"; line != want {
		t.Errorf("got %q, want %q", line, want)
	}

	if _, err := summarizeUnits(results[1:]); err == nil {
		t.Error("every season failing should be an error")
	}
}

// A favourite league is followed even when only old seasons are cached:
// starring it says "keep this current", whatever was downloaded before.
func TestStaleRefreshesFollowsFavouriteLeagues(t *testing.T) {
	repos := storagetest.New(t)
	seedSeason(t, repos, "ita-serie-a", "2016", september.Add(-24*time.Hour), models.SyncStatusOK)
	seedSeason(t, repos, "fra-ligue-1", "2015", september.Add(-24*time.Hour), models.SyncStatusOK)
	if err := repos.AddFavorite(context.Background(), models.Favorite{
		Kind: models.FavoriteCompetition, ID: "ita-serie-a", AddedAt: september}); err != nil {
		t.Fatal(err)
	}

	reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour)
	if len(reqs) != 1 || reqs[0].CompetitionID != "ita-serie-a" || reqs[0].Seasons[0] != "2026" {
		t.Fatalf("got %+v, want the favourite's current season only", reqs)
	}
	if reqs[0].Why != "not cached yet" {
		t.Errorf("reason %q", reqs[0].Why)
	}
}
