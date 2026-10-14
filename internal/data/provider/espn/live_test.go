package espn

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
	footysync "github.com/JohnSBarden/footui/internal/data/sync"
)

// TestLiveCanary refreshes the current NWSL season from ESPN's real API
// and checks every payload a refresh reads — standings, team list,
// rosters, team statistics — still maps into something usable. The API
// is undocumented and can change shape without notice; the fixture tests
// can't see that, so this does. It is opt-in (FOOTY_LIVE=1) so ordinary
// test runs stay offline, and it runs weekly in CI
// (.github/workflows/canary.yml).
//
// NWSL because it is one of the smallest leagues ESPN serves us: a
// refresh is two requests per club, so a small league keeps the canary
// to a few dozen throttled requests.
func TestLiveCanary(t *testing.T) {
	if os.Getenv("FOOTY_LIVE") != "1" {
		t.Skip("live provider check; set FOOTY_LIVE=1 to run")
	}
	// A cached payload would prove nothing about the live API.
	t.Setenv(CacheDirEnv, "")

	const compID = "usa-nwsl"
	cov, ok := footysync.FindCoverage(compID)
	if !ok || cov.Provider != footysync.ProviderESPN {
		t.Fatalf("%s is no longer routed to ESPN; pick another league for the canary", compID)
	}
	year := cov.CurrentSeason(time.Now())
	comp := provider.CompetitionRef{CompetitionID: compID, Name: cov.Competition.Name}
	season := provider.SeasonRef{SeasonID: compID + ":" + year, Label: year}
	t.Logf("espn %s %s", compID, year)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	p := New()

	rows, err := p.Standings(ctx, comp, season)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(rows) < 10 {
		t.Fatalf("got %d standings rows, want a full league", len(rows))
	}

	// The rest is served from the bundle already fetched.
	teams, err := p.Teams(ctx, comp, season)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != len(rows) {
		t.Errorf("%d teams for %d standings rows", len(teams), len(rows))
	}
	for _, tm := range teams {
		if tm.ID == "" || tm.Name == "" {
			t.Errorf("team missing identity: %+v", tm)
		}
	}

	var played, goals, ranked, withPossession, withShots int
	for _, r := range rows {
		played += r.MatchesPlayed
		goals += r.GoalsFor
		if r.Position != nil {
			ranked++
		}
		if r.PossessionAvg != nil {
			withPossession++
		}
		if r.Shots != nil {
			withShots++
		}
		if r.MatchesPlayed != r.Wins+r.Draws+r.Losses {
			t.Errorf("%s: %d played but %d-%d-%d", r.TeamID, r.MatchesPlayed, r.Wins, r.Draws, r.Losses)
		}
	}
	if played == 0 || goals == 0 {
		t.Errorf("no matches or goals in the table (played %d, goals %d)", played, goals)
	}
	if ranked != len(rows) {
		t.Errorf("only %d of %d clubs have a table position", ranked, len(rows))
	}
	// The statistics endpoint is fetched per club and its failures are
	// tolerated, so a broken shape there would otherwise pass silently.
	if withPossession == 0 || withShots == 0 {
		t.Errorf("team statistics no longer parse: possession for %d clubs, shots for %d", withPossession, withShots)
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
	var apps, scored int
	for _, l := range lines {
		apps += l.Appearances
		scored += l.Goals
	}
	if apps == 0 || scored == 0 {
		t.Errorf("player appearances %d, goals %d: roster stats no longer parse", apps, scored)
	}

	colors, err := p.TeamColors(ctx, comp, season)
	if err != nil {
		t.Fatal(err)
	}
	if len(colors) == 0 {
		t.Error("no team colors: the team list no longer parses")
	}
	t.Logf("%d clubs, %d matches played, %d goals; possession for %d clubs; %d players, %d colors",
		len(rows), played/2, goals, withPossession, len(players), len(colors))
}
