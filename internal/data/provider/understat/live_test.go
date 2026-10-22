package understat

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
	datasync "github.com/JohnSBarden/footui/internal/data/sync"
)

// TestLiveCanary fetches the current Premier League season from the real
// understat.com and checks the payload still maps into something usable.
// understat.com is undocumented and can change shape without notice; the
// fixture tests can't see that, so this does. It is opt-in (FOOTUI_LIVE=1)
// so ordinary test runs stay offline, and it runs weekly in CI
// (.github/workflows/canary.yml). One league, one request.
func TestLiveCanary(t *testing.T) {
	if os.Getenv("FOOTUI_LIVE") != "1" {
		t.Skip("live provider check; set FOOTUI_LIVE=1 to run")
	}
	// A cached payload would prove nothing about the live site.
	t.Setenv(CacheDirEnv, "")

	const compID = "eng-premier-league"
	cov, ok := datasync.FindCoverage(compID)
	if !ok || cov.Provider != datasync.ProviderUnderstat {
		t.Fatalf("%s is no longer routed to understat; pick another league for the canary", compID)
	}
	year := cov.CurrentSeason(time.Now())
	comp := provider.CompetitionRef{CompetitionID: compID, Name: cov.Competition.Name}
	season := provider.SeasonRef{SeasonID: compID + ":" + year, Label: year}
	t.Logf("understat %s %s", compID, year)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p := New()

	teams, err := p.Teams(ctx, comp, season)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(teams) < 18 {
		t.Fatalf("got %d teams, want a full league", len(teams))
	}
	for _, tm := range teams {
		if tm.ID == "" || tm.Name == "" {
			t.Errorf("team missing identity: %+v", tm)
		}
	}

	// The rest is served from the payload already fetched.
	rows, err := p.Standings(ctx, comp, season)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(teams) {
		t.Errorf("%d standings rows for %d teams", len(rows), len(teams))
	}
	var played, goals int
	var xg, xga float64
	for _, r := range rows {
		played += r.MatchesPlayed
		goals += r.GoalsFor
		if r.XG != nil {
			xg += *r.XG
		}
		if r.XGA != nil {
			xga += *r.XGA
		}
		if r.MatchesPlayed != r.Wins+r.Draws+r.Losses {
			t.Errorf("%s: %d played but %d-%d-%d", r.TeamID, r.MatchesPlayed, r.Wins, r.Draws, r.Losses)
		}
	}
	if played == 0 || goals == 0 {
		t.Errorf("no matches or goals in the table (played %d, goals %d)", played, goals)
	}
	if xg == 0 || xga == 0 {
		t.Errorf("league xG %.1f / xGA %.1f: expected goals no longer parse", xg, xga)
	}

	players, err := p.Players(ctx, comp, season)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := p.AllRosterStats(ctx, comp, season)
	if err != nil {
		t.Fatal(err)
	}
	if len(players) < 100 || len(lines) < 100 {
		t.Fatalf("got %d players, %d stat lines; want whole squads", len(players), len(lines))
	}
	for _, pl := range players {
		if pl.ID == "" || pl.Name == "" {
			t.Errorf("player missing identity: %+v", pl)
			break
		}
	}
	var minutes int
	var playerXG float64
	for _, l := range lines {
		minutes += l.Minutes
		if l.XG != nil {
			playerXG += *l.XG
		}
	}
	if minutes == 0 || playerXG == 0 {
		t.Errorf("player minutes %d, xG %.1f: player stats no longer parse", minutes, playerXG)
	}
	t.Logf("%d teams, %d matches played, %d goals, xG %.1f; %d players", len(teams), played/2, goals, xg, len(players))
}
