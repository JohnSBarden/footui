package cli

import (
	"context"
	"net/http"
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
		if err := repos.UpsertMatches(ctx, []models.Match{{ID: "understat-1", CompetitionID: pl.ID, SeasonID: season.ID,
			Kickoff: now.Add(-72 * time.Hour), Status: models.MatchFullTime, HomeTeamID: arsenal.ID, AwayTeamID: "eng-chelsea",
			HomeScore: new(1), AwayScore: new(0), Source: "understat", UpdatedAt: now}}); err != nil {
			t.Fatal(err)
		}
	}
	return arsenal
}

// A season stored only for a club's matches isn't followed.
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

// A starred club's recent games are due when its league has no matches
// stored (a database from before club matches).
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
	// The last game came from understat: looked for on its day and the
	// days either side, not found, and not looked for again.
	if days, sums := atomic.LoadInt32(&fake.days), atomic.LoadInt32(&fake.summaries); days != 3 || sums != 0 {
		t.Errorf("last game: %d day scoreboards, %d summaries; want 3 and none", days, sums)
	}
}

// A last game that failed to fetch rests ten minutes, as a failed unit
// does, rather than being asked for every time the Team screen opens.
func TestAFailedLastGameRests(t *testing.T) {
	t.Cleanup(func() { lastGameFailures.at = map[string]time.Time{} })
	fake := useFakeESPN(t)
	fake.dayStatus = http.StatusInternalServerError
	repos := withDirectory(t)
	now := time.Now()
	arsenal := seedArsenal(t, repos, now, time.Hour, true)

	refresh := tuiRefresher(repos, 6*time.Hour)
	req := core.RefreshRequest{TeamID: arsenal.ID, Name: arsenal.Name, Automatic: true}
	if _, err := refresh(context.Background(), req, func(string) {}); err != nil {
		t.Fatalf("a failed last game failed the refresh: %v", err)
	}
	if atomic.LoadInt32(&fake.days) == 0 {
		t.Fatal("the last game wasn't looked for")
	}
	if why, due := clubDue(context.Background(), repos, arsenal, now, 6*time.Hour); due {
		t.Errorf("due again at once (%s)", why)
	}
	asked := atomic.LoadInt32(&fake.days)
	if _, err := refresh(context.Background(), req, func(string) {}); err != nil || atomic.LoadInt32(&fake.days) != asked {
		t.Errorf("asked again within the rest: %v, %d requests", err, atomic.LoadInt32(&fake.days)-asked)
	}
	if why, due := clubDue(context.Background(), repos, arsenal, now.Add(11*time.Minute), 6*time.Hour); !due || why != "last game" {
		t.Errorf("not due after the rest: %v (%s)", due, why)
	}
}

func TestAClubSummaryNamesTheLastGame(t *testing.T) {
	for _, c := range []struct{ summary, lastGame, want string }{
		{"fetched 2, 14 matches", "last game in full", "fetched 2, 14 matches; last game in full"},
		{"recent games up to date", "last game in full", "last game in full"},
		{"nothing to fetch", "last game in full", "last game in full"},
		{"recent games up to date", "", "recent games up to date"},
	} {
		if got := withLastGame(c.summary, c.lastGame); got != c.want {
			t.Errorf("withLastGame(%q, %q) = %q, want %q", c.summary, c.lastGame, got, c.want)
		}
	}
}
