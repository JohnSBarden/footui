package espn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	footysync "github.com/JohnSBarden/footui/internal/data/sync"
)

var (
	mls      = provider.CompetitionRef{CompetitionID: "usa-mls", Name: "MLS"}
	mls2025  = provider.SeasonRef{SeasonID: "usa-mls:2025", Label: "2025"}
	ctxBkgnd = context.Background()
)

// fixtureServer serves the payloads in testdata for the URLs the provider
// builds, and 404s anything else (including rosters and stats for the
// clubs that have none there, which the provider must tolerate). The
// payloads are hand-written in ESPN's shape, with fictional clubs and
// players, and carry only the fields the provider reads.
func fixtureServer(t *testing.T) (*Provider, *int32) {
	t.Helper()
	var requests int32
	routes := map[string]string{
		"/site/v2/sports/soccer/usa.1/standings":                "standings_usa.1_2025.json",
		"/site/site/v2/sports/soccer/usa.1/teams":               "teams_usa.1.json",
		"/site/site/v2/sports/soccer/usa.1/teams/182/roster":    "roster_usa.1_182_2025.json",
		"/core/usa.1/seasons/2025/types/1/teams/182/statistics": "teamstats_usa.1_182_2025.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		file, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return newWithClient(NewClient().withBaseURLs(srv.URL+"/site", srv.URL+"/core")), &requests
}

func TestStandingsMapGroupsAndGoalDifference(t *testing.T) {
	p, _ := fixtureServer(t)
	rows, err := p.Standings(ctxBkgnd, mls, mls2025)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want both conferences' clubs", len(rows))
	}
	lakeshore := find(t, rows, "espn:182")
	if lakeshore.MatchesPlayed != 34 || lakeshore.Wins != 15 || lakeshore.Draws != 8 || lakeshore.Losses != 11 {
		t.Errorf("record %d-%d-%d in %d", lakeshore.Wins, lakeshore.Draws, lakeshore.Losses, lakeshore.MatchesPlayed)
	}
	if lakeshore.GoalsFor != 68 || lakeshore.GoalsAgainst != 60 || lakeshore.GoalDiff != 8 || lakeshore.Points != 53 {
		t.Errorf("goals/points %+v", lakeshore)
	}
	// Two conferences become one table, not two tables each numbered from
	// 1: Riverside 54 pts, Lakeshore 53, Prairie City 44, Highland 41.
	for id, want := range map[string]int{"espn:183": 1, "espn:182": 2, "espn:185": 3, "espn:184": 4} {
		if got := find(t, rows, id).Position; got == nil || *got != want {
			t.Errorf("%s position %v, want %d overall", id, got, want)
		}
	}
	if lakeshore.XG != nil {
		t.Error("ESPN has no expected goals; xG must stay unset")
	}
	// Each club still knows its conference, for the screen's tabs.
	groups := map[string]bool{}
	for _, r := range rows {
		groups[r.GroupLabel] = true
	}
	if !groups["Eastern Conference"] || !groups["Western Conference"] || len(groups) != 2 {
		t.Errorf("conference labels %v", groups)
	}
}

func find(t *testing.T, rows []models.TeamSeasonStats, id string) models.TeamSeasonStats {
	t.Helper()
	for _, r := range rows {
		if r.TeamID == id {
			return r
		}
	}
	t.Fatalf("no row for %s", id)
	return models.TeamSeasonStats{}
}

// The fixture's Lakeshore season reports 0% possession and 1,856 corners
// in 34 matches, as real ESPN seasons sometimes do. Neither can be true,
// so neither is stored; the plausible figures beside them are.
func TestTeamStatsKeepThePlausibleAndDropTheImpossible(t *testing.T) {
	p, _ := fixtureServer(t)
	rows, _ := p.Standings(ctxBkgnd, mls, mls2025)
	club := find(t, rows, "espn:182")

	if club.PossessionAvg != nil {
		t.Errorf("possession %v: 0%% means unknown, not zero", *club.PossessionAvg)
	}
	if club.Corners != nil {
		t.Errorf("corners %d in 34 matches is not plausible", *club.Corners)
	}
	for name, got := range map[string]*int{"fouls": club.FoulsCommitted, "shots": club.Shots, "on target": club.ShotsOnTarget, "yellows": club.YellowCards, "reds": club.RedCards} {
		if got == nil {
			t.Errorf("%s should be kept", name)
		}
	}
	if *club.Shots != 512 || *club.ShotsOnTarget != 208 {
		t.Errorf("shots %d/%d", *club.Shots, *club.ShotsOnTarget)
	}
	// A club whose statistics 404 simply has none.
	if riverside := find(t, rows, "espn:183"); riverside.Shots != nil {
		t.Error("missing statistics should leave the columns unset")
	}
}

func TestRostersKeepOnlyPlayersWhoAppeared(t *testing.T) {
	p, _ := fixtureServer(t)
	players, err := p.Players(ctxBkgnd, mls, mls2025)
	if err != nil {
		t.Fatal(err)
	}
	lines, _ := p.AllRosterStats(ctxBkgnd, mls, mls2025)
	if len(players) != 3 || len(lines) != 3 {
		t.Fatalf("got %d players, %d lines", len(players), len(lines))
	}
	striker := players[0]
	if striker.ID != "espn:900001" || striker.Name != "Pim Vandersloot" || striker.Position != "F" || striker.Gender != "men" {
		t.Errorf("player %+v", striker)
	}
	if striker.DateOfBirth == nil {
		t.Error("date of birth should parse")
	}
	l := lines[0]
	if l.TeamID != "espn:182" || l.Appearances != 36 || l.Goals != 19 || l.Assists != 2 || l.Shots == nil || *l.Shots != 89 {
		t.Errorf("stat line %+v", l)
	}
	if l.Minutes != 0 || l.XG != nil {
		t.Error("ESPN has no minutes or xG; they must stay unset")
	}
}

func TestColorsAreNormalisedHex(t *testing.T) {
	p, _ := fixtureServer(t)
	colors, err := p.TeamColors(ctxBkgnd, mls, mls2025)
	if err != nil {
		t.Fatal(err)
	}
	var lakeshore models.TeamColors
	for _, c := range colors {
		if c.TeamID == "espn:182" {
			lakeshore = c
		}
	}
	if lakeshore.PrimaryHex != "#7CCDEF" || lakeshore.SecondaryHex != "#FF0000" || lakeshore.Source != "espn" {
		t.Errorf("colors %+v", lakeshore)
	}
}

func TestOneFetchServesEveryMethod(t *testing.T) {
	p, requests := fixtureServer(t)
	_, _ = p.Teams(ctxBkgnd, mls, mls2025)
	before := atomic.LoadInt32(requests)
	_, _ = p.Standings(ctxBkgnd, mls, mls2025)
	_, _ = p.Players(ctxBkgnd, mls, mls2025)
	_, _ = p.AllRosterStats(ctxBkgnd, mls, mls2025)
	if after := atomic.LoadInt32(requests); after != before {
		t.Errorf("%d extra requests after the first call; one fetch should serve them all", after-before)
	}
}

func TestASeasonWithNoTableIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"children": []}`))
	}))
	defer srv.Close()
	p := newWithClient(NewClient().withBaseURLs(srv.URL, srv.URL))
	_, err := p.Standings(ctxBkgnd, mls, provider.SeasonRef{SeasonID: "usa-mls:1990", Label: "1990"})
	if !errors.Is(err, ErrNoTable) {
		t.Errorf("got %v, want ErrNoTable", err)
	}
}

func TestServerErrorsAreRetriedAndNotFoundIsNot(t *testing.T) {
	for _, tc := range []struct {
		status   int
		attempts int32
	}{{http.StatusInternalServerError, maxAttempts}, {http.StatusNotFound, 1}} {
		var hits int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hits, 1)
			w.WriteHeader(tc.status)
		}))
		c := NewClient().withBaseURLs(srv.URL, srv.URL)
		var retries int
		c.OnRetry = func(int, int, error) { retries++ }
		var out any
		err := c.site(ctxBkgnd, "/x", &out)
		srv.Close()
		if err == nil {
			t.Errorf("HTTP %d: expected an error", tc.status)
		}
		if hits != tc.attempts {
			t.Errorf("HTTP %d: %d attempts, want %d", tc.status, hits, tc.attempts)
		}
		if tc.status == http.StatusNotFound && !errors.Is(err, ErrNotFound) {
			t.Errorf("404 should be ErrNotFound, got %v", err)
		}
	}
}

func TestDiskCacheServesRepeatRequests(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()
	c := NewClient().withBaseURLs(srv.URL, srv.URL)
	c.cacheDir = t.TempDir()
	var out map[string]bool
	for i := 0; i < 3; i++ {
		if err := c.site(ctxBkgnd, "/same", &out); err != nil || !out["ok"] {
			t.Fatalf("fetch %d: %v %v", i, out, err)
		}
	}
	if hits != 1 {
		t.Errorf("%d network requests for three identical fetches with a cache", hits)
	}
}

// Every competition the registry routes here must have a slug, or a
// refresh of it fails at runtime instead of in this test.
func TestEveryRoutedCompetitionHasASlug(t *testing.T) {
	for _, c := range footysync.Coverages {
		if c.Provider != footysync.ProviderESPN {
			continue
		}
		if _, ok := leagueSlugs[c.Competition.ID]; !ok {
			t.Errorf("%s is routed to espn but has no leagueSlugs entry", c.Competition.ID)
		}
	}
	for id := range leagueSlugs {
		if c, ok := footysync.FindCoverage(id); !ok || c.Provider != footysync.ProviderESPN {
			t.Errorf("leagueSlugs has %s, which the registry doesn't route to espn", id)
		}
	}
	if strings.Contains(teamID("1"), " ") {
		t.Error("ids must not contain spaces")
	}
}
