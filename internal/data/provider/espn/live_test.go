package espn

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
	datasync "github.com/JohnSBarden/footui/internal/data/sync"
)

// TestLiveCanary refreshes the current NWSL season (a small league, so few
// requests) from the real API, catching shape changes fixtures can't.
// Opt-in via FOOTUI_LIVE=1; runs weekly in .github/workflows/canary.yml.
func TestLiveCanary(t *testing.T) {
	if os.Getenv("FOOTUI_LIVE") != "1" {
		t.Skip("live provider check; set FOOTUI_LIVE=1 to run")
	}
	// A cached payload would prove nothing about the live API.
	t.Setenv(CacheDirEnv, "")

	const compID = "usa-nwsl"
	cov, ok := datasync.FindCoverage(compID)
	if !ok || cov.Provider != datasync.ProviderESPN {
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

// TestLivePlayerSearch checks search still finds an uncached player and
// his career seasons.
func TestLivePlayerSearch(t *testing.T) {
	if os.Getenv("FOOTUI_LIVE") != "1" {
		t.Skip("live provider check; set FOOTUI_LIVE=1 to run")
	}
	t.Setenv(CacheDirEnv, "")
	p := New()
	found, err := p.SearchPlayers(context.Background(), "cristiano ronaldo")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 || found[0].Name != "Cristiano Ronaldo" {
		t.Fatalf("search no longer finds Cristiano Ronaldo first: %+v", found)
	}
	career, err := p.PlayerCareer(context.Background(), found[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(career) < 10 {
		t.Errorf("only %d seasons of Ronaldo's career found: %+v", len(career), career)
	}
	t.Logf("%s (%s): %d seasons in covered competitions", found[0].Name, found[0].Context, len(career))
}
