package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// A result that lands for a followed club goes on the ticker after the
// batch's summary, and the ticker turns between them.
func TestAFollowedClubsResultGoesOnTheTicker(t *testing.T) {
	ctx := context.Background()
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2026", storagetest.SampleStandings())
	if err := repos.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: storagetest.Fixed}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)
	app := NewApp(core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain()}, newFake("Browse"))
	app.clock = func() time.Time { return now }

	app.snapshotNews()
	if err := repos.UpsertMatches(ctx, []models.Match{{
		ID: "m1", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2026",
		Kickoff: now.Add(-3 * time.Hour), Status: models.MatchFullTime,
		HomeTeamID: "eng-luton-town", AwayTeamID: "eng-arsenal",
		HomeScore: new(0), AwayScore: new(4), Source: "test", UpdatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	app.status = "Refreshed Premier League: 20 teams"
	if cmd := app.newsSince(); cmd == nil {
		t.Fatal("news should start the ticker turning")
	}
	if app.status != "" {
		t.Errorf("the summary should move onto the ticker, status is %q", app.status)
	}
	if got := app.statusLine(); !strings.HasPrefix(got, "Refreshed Premier League") {
		t.Errorf("the ticker should lead with the summary, got %q", got)
	}
	app.Update(tickerMsg{})
	if got, want := app.statusLine(), "FT  Luton Town 0–4 Arsenal · Premier League  2/2"; got != want {
		t.Errorf("status line = %q, want %q", got, want)
	}

	// Once it's all stale, the ticker stops.
	now = now.Add(time.Hour)
	if _, cmd := app.Update(tickerMsg{}); cmd != nil || app.newsTurning || app.statusLine() != "" {
		t.Errorf("a stale ticker should stop and clear, line %q", app.statusLine())
	}
}
