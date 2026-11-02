package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider/espn"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// withDirectory is an empty store with one nation already in it, so the
// stale check doesn't also ask for the directory.
func withDirectory(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	if err := repos.UpsertTeam(context.Background(), models.Team{ID: "nat-arcadia", Name: "Arcadia", Gender: "men", National: true}); err != nil {
		t.Fatal(err)
	}
	return repos
}

// fakeESPN serves fictional nations in ESPN's shape, counting requests
// by kind. Every competition-year has one match: Arcadia v Borduria.
type fakeESPN struct {
	scoreboards, directories, standings int32
	// days and summaries are the last-game lookups: a day's scoreboard,
	// and a match summary.
	days, summaries int32
	status          int // non-zero: answer every request with it
	dayStatus       int // non-zero: answer a day's scoreboard with it
}

func (f *fakeESPN) serve(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/scoreboard") && len(r.URL.Query().Get("dates")) == len("20060102"):
			// A day's games, where a club's last game is looked for.
			atomic.AddInt32(&f.days, 1)
			if f.dayStatus != 0 {
				w.WriteHeader(f.dayStatus)
				return
			}
			fmt.Fprint(w, `{"events":[]}`)
		case strings.HasSuffix(r.URL.Path, "/summary"):
			atomic.AddInt32(&f.summaries, 1)
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/scoreboard"):
			n := atomic.AddInt32(&f.scoreboards, 1)
			dates := r.URL.Query().Get("dates")
			fmt.Fprintf(w, `{"events":[{"id":"%d","date":"%s-03-20T19:00Z","season":{"year":%s},"competitions":[{"status":{"type":{"name":"STATUS_FULL_TIME"}},`+
				`"competitors":[{"homeAway":"home","score":"2","team":{"id":"1","displayName":"Arcadia"}},{"homeAway":"away","score":"1","team":{"id":"2","displayName":"Borduria"}}]}]}]}`,
				n, dates, dates)
		case strings.HasSuffix(r.URL.Path, "/teams"):
			atomic.AddInt32(&f.directories, 1)
			fmt.Fprint(w, `{"sports":[{"leagues":[{"teams":[{"team":{"id":"1","displayName":"Arcadia"}},{"team":{"id":"2","displayName":"Borduria"}},{"team":{"id":"3","displayName":"Carpania"}}]}]}]}`)
		case strings.HasSuffix(r.URL.Path, "/standings"):
			atomic.AddInt32(&f.standings, 1)
			fmt.Fprint(w, `{"children":[{"name":"Group A","standings":{"entries":[{"team":{"id":"1","displayName":"Arcadia"},"stats":[{"name":"rank","value":1},{"name":"points","value":3}]}]}},`+
				`{"name":"Group B","standings":{"entries":[{"team":{"id":"2","displayName":"Borduria"},"stats":[{"name":"rank","value":1},{"name":"points","value":3}]}]}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// useFakeESPN points every international refresh at the fake.
func useFakeESPN(t *testing.T) *fakeESPN {
	t.Helper()
	f := &fakeESPN{}
	url := f.serve(t)
	old, oldLast := newMatchSource, newLastGameSource
	newMatchSource = func(refreshEvents) sync.MatchSource { return espn.NewAt(url) }
	newLastGameSource = func(refreshEvents) sync.LastGameSource { return espn.NewAt(url) }
	t.Cleanup(func() { newMatchSource, newLastGameSource = old, oldLast })
	return f
}

// The success criterion, end to end through the real ESPN provider: the
// directory, then starring one nation, costs about thirty requests; a
// second nation costs none.
func TestFollowingNationsEndToEndRequestBudget(t *testing.T) {
	f := useFakeESPN(t)
	repos := storagetest.New(t)
	refresh := tuiRefresher(repos, 6*time.Hour)
	ctx := context.Background()
	noProgress := func(string) {}

	if _, err := refresh(ctx, core.RefreshRequest{Directory: true}, noProgress); err != nil {
		t.Fatal(err)
	}
	if f.directories != 4 {
		t.Errorf("%d directory requests, want the men's list and the women's three", f.directories)
	}
	summary, err := refresh(ctx, core.RefreshRequest{TeamID: "nat-arcadia"}, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	units := len(sync.NationUnits("men", sync.RecentYears(time.Now(), 2)))
	if int(f.scoreboards) != units || units > 30 {
		t.Errorf("%d scoreboard requests for %d units; want one each, at most 30", f.scoreboards, units)
	}
	if !strings.Contains(summary, fmt.Sprintf("fetched %d", units)) {
		t.Errorf("summary %q", summary)
	}

	before := f.scoreboards
	summary, err = refresh(ctx, core.RefreshRequest{TeamID: "nat-borduria"}, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	if f.scoreboards != before || summary != "already up to date" {
		t.Errorf("second nation: %d more requests, summary %q", f.scoreboards-before, summary)
	}
	ms, _ := repos.MatchesForTeam(ctx, "nat-borduria", models.MatchFilter{}, nil)
	if len(ms) != units {
		t.Errorf("Borduria has %d matches, want one per unit", len(ms))
	}
}

func TestATournamentEditionIsOneScoreboardAndOneTable(t *testing.T) {
	f := useFakeESPN(t)
	repos := storagetest.New(t)
	refresh := tuiRefresher(repos, 6*time.Hour)
	// The fake's 2022 has no "final", so the next year is asked too; the
	// table is one request.
	if _, err := refresh(context.Background(), core.RefreshRequest{CompetitionID: "world-cup", Seasons: []string{"2022"}}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if f.standings != 1 || f.scoreboards != 2 {
		t.Errorf("%d scoreboards, %d standings", f.scoreboards, f.standings)
	}
	table, _ := repos.Standings(context.Background(), "world-cup", "world-cup:2022")
	if len(table) != 2 || table[0].GroupLabel != "Group A" || table[0].TeamID != "nat-arcadia" {
		t.Errorf("table %+v", table)
	}
}

// A 429 surfaces as core.ErrRateLimited, which is what makes the app drop
// its queue.
func TestRateLimitingReachesTheTUIAsErrRateLimited(t *testing.T) {
	f := useFakeESPN(t)
	f.status = http.StatusTooManyRequests
	repos := storagetest.New(t)
	_, err := tuiRefresher(repos, 0)(context.Background(), core.RefreshRequest{Nations: "men", Seasons: []string{"2025"}}, func(string) {})
	if !errors.Is(err, core.ErrRateLimited) {
		t.Fatalf("got %v, want core.ErrRateLimited", err)
	}
	if f.scoreboards != 0 {
		t.Error("counted scoreboards that were refused")
	}
	if _, err := repos.LastRateLimited(context.Background()); err != nil {
		t.Errorf("the refusal should be logged: %v", err)
	}
}

func TestStaleCheckAsksForTheDirectoryOnce(t *testing.T) {
	repos := storagetest.New(t)
	reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour)
	if len(reqs) != 1 || !reqs[0].Directory || !reqs[0].Automatic {
		t.Fatalf("got %+v, want the directory", reqs)
	}
	repos = withDirectory(t)
	if reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour); len(reqs) != 0 {
		t.Errorf("got %+v, want nothing once nations are cached", reqs)
	}
}

func TestStaleCheckFollowsStarredNationsAndTournaments(t *testing.T) {
	repos := withDirectory(t)
	ctx := context.Background()
	for _, f := range []models.Favorite{
		{Kind: models.FavoriteTeam, ID: "nat-arcadia", AddedAt: september},
		{Kind: models.FavoriteCompetition, ID: "euro", AddedAt: september},
	} {
		if err := repos.AddFavorite(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	reqs := staleRefreshes(ctx, repos, september, 6*time.Hour)
	if len(reqs) != 2 {
		t.Fatalf("got %+v", reqs)
	}
	if reqs[0].CompetitionID != "euro" || reqs[0].Seasons[0] != "2024" {
		t.Errorf("tournament request %+v, want Euro 2024", reqs[0])
	}
	if reqs[1].TeamID != "nat-arcadia" || reqs[1].Name != "Arcadia" || strings.Join(reqs[1].Seasons, " ") != "2026 2025" {
		t.Errorf("nation request %+v", reqs[1])
	}

	// Once every unit is fetched, after the years in question, nothing.
	for _, u := range sync.NationUnits("men", []int{2026, 2025}) {
		mustRecordYear(t, repos, u.Competition.ID, u.Year, september.Add(-time.Hour))
	}
	mustRecordYear(t, repos, "euro", 2024, september.Add(-time.Hour))
	if reqs := staleRefreshes(ctx, repos, september, 6*time.Hour); len(reqs) != 0 {
		t.Errorf("got %+v, want nothing due", reqs)
	}
}

func TestARecentRateLimitSuspendsTheStaleCheck(t *testing.T) {
	repos := storagetest.New(t)
	seedSeason(t, repos, "eng-premier-league", "2026", september.Add(-13*24*time.Hour), models.SyncStatusOK)
	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{Provider: "espn", CompetitionID: "int-friendly",
		Year: 2026, StartedAt: september.Add(-20 * time.Minute), FinishedAt: september.Add(-20 * time.Minute),
		Status: models.SyncStatusRateLimited}); err != nil {
		t.Fatal(err)
	}
	if reqs := staleRefreshes(context.Background(), repos, september, 6*time.Hour); len(reqs) != 0 {
		t.Errorf("got %+v, want nothing within the hour", reqs)
	}
	if reqs := staleRefreshes(context.Background(), repos, september.Add(2*time.Hour), 6*time.Hour); len(reqs) == 0 {
		t.Error("the pause should end")
	}
}

func mustRecordYear(t *testing.T, repos storage.Repositories, comp string, year int, at time.Time) {
	t.Helper()
	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{Provider: "espn", CompetitionID: comp, Year: year,
		StartedAt: at, FinishedAt: at, Status: models.SyncStatusOK}); err != nil {
		t.Fatal(err)
	}
}

func TestParseNationQuery(t *testing.T) {
	for q, want := range map[string]string{
		"england":         "england/men",
		"england women":   "england/women",
		"England Women's": "England/women",
		"south korea":     "south korea/men",
		"women":           "women/men",
	} {
		name, gender := parseNationQuery(q)
		if got := name + "/" + gender; got != want {
			t.Errorf("%q: %s, want %s", q, got, want)
		}
	}
}

func TestResolveNationSeedsTheDirectoryFirst(t *testing.T) {
	repos := storagetest.New(t)
	seeded := 0
	seed := func() error {
		seeded++
		for _, n := range []models.Team{
			{ID: "nat-england", Name: "England", Gender: "men", National: true},
			{ID: "nat-england-women", Name: "England", Gender: "women", National: true},
			{ID: "nat-new-zealand", Name: "New Zealand", Gender: "men", National: true},
			{ID: "nat-new-caledonia", Name: "New Caledonia", Gender: "men", National: true},
		} {
			if err := repos.UpsertTeam(context.Background(), n); err != nil {
				return err
			}
		}
		return nil
	}
	team, err := resolveNation(context.Background(), repos, "England Women", seed)
	if err != nil || team.ID != "nat-england-women" || seeded != 1 {
		t.Errorf("got %+v, %v (seeded %d)", team, err, seeded)
	}
	if team, err := resolveNation(context.Background(), repos, "zealand", seed); err != nil || team.ID != "nat-new-zealand" || seeded != 1 {
		t.Errorf("got %+v, %v: a unique substring is enough, and the directory isn't fetched twice", team, err)
	}
	if _, err := resolveNation(context.Background(), repos, "new", seed); err == nil || !strings.Contains(err.Error(), "New Caledonia") {
		t.Errorf("an ambiguous name should list the options: %v", err)
	}
}

func TestResolveCompetitionPrefersAnExactID(t *testing.T) {
	repos := storagetest.New(t)
	for _, id := range []string{"world-cup", "world-cup-women"} {
		c, _ := sync.FindCompetition(id)
		if err := repos.UpsertCompetition(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	c, err := resolveCompetition(context.Background(), repos, "world-cup")
	if err != nil || c.ID != "world-cup" {
		t.Errorf("got %+v, %v", c, err)
	}
}
