package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/provider/espn"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

type stubDetail struct{ err error }

func (s stubDetail) HasDetail(string) bool { return true }
func (s stubDetail) AthleteSeason(context.Context, string, int, string, bool) (models.PlayerSeasonDetail, error) {
	if s.err != nil {
		return models.PlayerSeasonDetail{}, s.err
	}
	return models.PlayerSeasonDetail{CleanSheets: new(3)}, nil
}
func (s stubDetail) Squad(context.Context, string, int, string) ([]provider.SquadMember, error) {
	return nil, nil
}

func useDetail(t *testing.T, src sync.DetailSource) {
	t.Helper()
	old := newDetailSource
	newDetailSource = func() sync.DetailSource { return src }
	t.Cleanup(func() { newDetailSource = old })
}

// seedRaya caches a Premier League keeper whose ESPN ID is known.
func seedRaya(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	ctx := context.Background()
	seedSeason(t, repos, "eng-premier-league", "2026", time.Time{}, "")
	for _, err := range []error{
		repos.UpsertPlayer(ctx, models.Player{ID: "understat-9676", Name: "David Raya", Position: "GK"}),
		repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{PlayerID: "understat-9676", TeamID: "eng-arsenal",
			CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2026", Minutes: 540, Source: "understat"}),
		repos.PutExternalRef(ctx, "player", "understat-9676", "espn", "196176"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	return repos
}

func TestPlayerRefreshFetchesDetail(t *testing.T) {
	useDetail(t, stubDetail{})
	repos := seedRaya(t)
	summary, err := tuiRefresher(repos, 6*time.Hour)(context.Background(),
		core.RefreshRequest{PlayerID: "understat-9676", Name: "David Raya"}, func(string) {})
	if err != nil || summary != "1 season of stats refreshed" {
		t.Fatalf("summary %q, err %v", summary, err)
	}
	rows, _ := repos.PlayerSeasonDetails(context.Background(), "understat-9676")
	if len(rows) != 1 {
		t.Errorf("%d detail rows", len(rows))
	}
}

func TestPlayerRefreshRateLimitedReturnsErr(t *testing.T) {
	useDetail(t, stubDetail{err: provider.ErrRateLimited})
	repos := seedRaya(t)
	_, err := tuiRefresher(repos, 6*time.Hour)(context.Background(),
		core.RefreshRequest{PlayerID: "understat-9676"}, func(string) {})
	if !errors.Is(err, core.ErrRateLimited) {
		t.Errorf("err = %v, want core.ErrRateLimited", err)
	}
}

type stubCareer []provider.CareerSeason

func (s stubCareer) AthleteCareer(context.Context, string) ([]provider.CareerSeason, error) {
	return s, nil
}

func useCareer(t *testing.T, src sync.CareerSource) {
	t.Helper()
	old := newCareerSource
	newCareerSource = func() sync.CareerSource { return src }
	t.Cleanup(func() { newCareerSource = old })
}

// r on a player asks for their whole career. A finished season fetched
// after it ended is kept rather than fetched again (so this test makes no
// request for it), the cup isn't a table, and the detail comes after.
func TestPlayerHistoryRefresh(t *testing.T) {
	useDetail(t, stubDetail{})
	useCareer(t, stubCareer{
		{CompetitionID: "eng-premier-league", Year: 2023},
		{CompetitionID: "eng-fa-cup", Year: 2023},
	})
	repos := seedRaya(t)
	seedSeason(t, repos, "eng-premier-league", "2023", time.Now(), models.SyncStatusOK)

	var lines []string
	summary, err := tuiRefresher(repos, 6*time.Hour)(context.Background(),
		core.RefreshRequest{PlayerID: "understat-9676", Name: "David Raya", History: true},
		func(s string) { lines = append(lines, s) })
	if err != nil || summary != "1 already cached; 1 season of stats refreshed" {
		t.Fatalf("summary %q, err %v", summary, err)
	}
	if len(lines) == 0 || !strings.Contains(lines[0], "(1 of 1)") {
		t.Errorf("progress = %q", lines)
	}
}

// A player ESPN doesn't know still gets their cached seasons' detail.
func TestPlayerHistoryRefreshWithoutAnESPNID(t *testing.T) {
	useDetail(t, stubDetail{})
	useCareer(t, stubCareer{})
	repos := storagetest.New(t)
	ctx := context.Background()
	seedSeason(t, repos, "eng-premier-league", "2026", time.Time{}, "")
	if err := repos.UpsertPlayer(ctx, models.Player{ID: "understat-1", Name: "Nobody", Position: "M"}); err != nil {
		t.Fatal(err)
	}
	summary, err := tuiRefresher(repos, 6*time.Hour)(ctx,
		core.RefreshRequest{PlayerID: "understat-1", History: true}, func(string) {})
	if err != nil || !strings.Contains(summary, "couldn't be looked up") {
		t.Fatalf("summary %q, err %v", summary, err)
	}
}

func TestStartupQueuesFavouritePlayers(t *testing.T) {
	repos := seedRaya(t)
	if err := repos.AddFavorite(context.Background(), models.Favorite{Kind: models.FavoritePlayer, ID: "understat-9676", AddedAt: september}); err != nil {
		t.Fatal(err)
	}
	var got *core.RefreshRequest
	for _, r := range staleRefreshes(context.Background(), repos, september, 6*time.Hour) {
		if r.PlayerID != "" {
			got = &r
		}
	}
	if got == nil || got.PlayerID != "understat-9676" || got.Name != "David Raya" || !got.Automatic {
		t.Fatalf("favourite player request = %+v", got)
	}
}

// Refresh failures reach the TUI's status line, which never names a
// data source.
func TestTUIErrorsNameNoSource(t *testing.T) {
	for _, err := range []error{
		&understat.TimeoutError{Host: "understat.com", After: 20 * time.Second},
		&espn.TimeoutError{After: 20 * time.Second},
		espn.ErrNoTable,
	} {
		msg := strings.ToLower(tuiError(err).Error())
		if strings.Contains(msg, "espn") || strings.Contains(msg, "understat") {
			t.Errorf("%q names a source", msg)
		}
	}
}
