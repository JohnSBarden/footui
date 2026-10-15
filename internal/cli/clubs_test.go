package cli

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// seedArsenal caches Arsenal's current Premier League season, refreshed
// syncedAgo before now, with one league match stored when withMatch.
func seedArsenal(t *testing.T, repos storage.Repositories, now time.Time, syncedAgo time.Duration, withMatch bool) models.Team {
	t.Helper()
	ctx := context.Background()
	pl, _ := sync.FindCompetition("eng-premier-league")
	season := sync.NewSeason(pl.ID, "2026")
	arsenal := models.Team{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}
	for _, err := range []error{
		repos.UpsertCompetition(ctx, pl),
		repos.UpsertSeason(ctx, season),
		repos.UpsertTeam(ctx, arsenal),
		repos.UpsertTeam(ctx, models.Team{ID: "eng-chelsea", Name: "Chelsea", Country: "England", Gender: "men"}),
		repos.LinkTeamCompetitionSeason(ctx, arsenal.ID, pl.ID, season.ID),
		repos.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{TeamID: arsenal.ID, CompetitionID: pl.ID,
			SeasonID: season.ID, MatchesPlayed: 7, Source: "understat", UpdatedAt: now}),
		repos.RecordSyncRun(ctx, models.SyncRun{Provider: "understat", CompetitionID: pl.ID, SeasonID: season.ID,
			StartedAt: now.Add(-syncedAgo), FinishedAt: now.Add(-syncedAgo), Status: models.SyncStatusOK}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if withMatch {
		one, nil0 := 1, 0
		if err := repos.UpsertMatches(ctx, []models.Match{{ID: "understat-1", CompetitionID: pl.ID, SeasonID: season.ID,
			Kickoff: now.Add(-72 * time.Hour), Status: models.MatchFullTime, HomeTeamID: arsenal.ID, AwayTeamID: "eng-chelsea",
			HomeScore: &one, AwayScore: &nil0, Source: "understat", UpdatedAt: now}}); err != nil {
			t.Fatal(err)
		}
	}
	return arsenal
}

// A Champions League or cup season stored only because a club's matches
// came from it is not a competition anyone follows.
func TestTheStaleCheckIgnoresCompetitionsHeldOnlyForMatches(t *testing.T) {
	repos := withDirectory(t)
	ctx := context.Background()
	for _, id := range []string{"eur-champions-league", "eng-fa-cup"} {
		c, _ := sync.FindCompetition(id)
		if err := repos.UpsertCompetition(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := repos.UpsertSeason(ctx, sync.NewSeason(id, "2026")); err != nil {
			t.Fatal(err)
		}
	}
	if reqs := staleRefreshes(ctx, repos, september, 6*time.Hour); len(reqs) != 0 {
		t.Errorf("queued %+v", reqs)
	}
}

// A starred club's recent games are followed on open: a database from
// before club matches has none for its league, so they are due.
func TestAStarredClubIsFollowedForItsRecentGames(t *testing.T) {
	repos := withDirectory(t)
	ctx := context.Background()
	seedArsenal(t, repos, september, time.Hour, false)
	if err := repos.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: september}); err != nil {
		t.Fatal(err)
	}
	var club *core.RefreshRequest
	for _, r := range staleRefreshes(ctx, repos, september, 6*time.Hour) {
		if r.TeamID == "eng-arsenal" {
			club = &r
		}
	}
	if club == nil || !club.Automatic || club.Why != "recent league games" {
		t.Fatalf("no automatic refresh of Arsenal's recent games: %+v", club)
	}
}

// The refresh the Team screen queues: the shared units of England's cups
// and Europe, once — a second ask fetches nothing.
func TestAClubRefreshFetchesItsUnitsOnce(t *testing.T) {
	fake := useFakeESPN(t)
	repos := withDirectory(t)
	now := time.Now()
	arsenal := seedArsenal(t, repos, now, time.Hour, true)

	if why, due := clubDue(context.Background(), repos, arsenal, now, 6*time.Hour); !due || !strings.Contains(why, "not fetched yet") {
		t.Errorf("due %v (%s): its cups and European games were never fetched", due, why)
	}
	refresh := tuiRefresher(repos, 6*time.Hour)
	req := core.RefreshRequest{TeamID: arsenal.ID, Name: arsenal.Name, Automatic: true}
	summary, err := refresh(context.Background(), req, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&fake.scoreboards) != 10 || !strings.HasPrefix(summary, "fetched 10") {
		t.Errorf("%d scoreboard requests, summary %q; want England's two cups and Europe's three competitions, two years each",
			atomic.LoadInt32(&fake.scoreboards), summary)
	}
	summary, err = refresh(context.Background(), req, func(string) {})
	if err != nil || atomic.LoadInt32(&fake.scoreboards) != 10 || summary != "recent games up to date" {
		t.Errorf("second refresh: %q, %v, %d requests", summary, err, atomic.LoadInt32(&fake.scoreboards))
	}
	if _, due := clubDue(context.Background(), repos, arsenal, now, 6*time.Hour); due {
		t.Error("still due after a refresh")
	}
}
