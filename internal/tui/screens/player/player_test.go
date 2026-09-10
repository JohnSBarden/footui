package player_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var haaland = models.Player{ID: "p-haaland", Name: "Erling Haaland", Position: "F S", Gender: "men"}

func newDeps(repos storage.Repositories) core.Deps {
	return core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
}

func settled(t *testing.T, deps core.Deps, teamID string) core.Screen {
	t.Helper()
	s := tuitest.Settle(t, player.New(deps, haaland, teamID))
	return tuitest.Resize(t, s, 112, 34)
}

// seedTwoCompetitions gives Haaland a league season and a cup season with
// deliberately different numbers, so career totals can be checked for
// blending.
func seedTwoCompetitions(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	city := []storagetest.TeamRow{{ID: "eng-manchester-city", Name: "Manchester City", Position: 1, MP: 38, Pts: 88, GF: 96, GA: 34}}

	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", city)
	storagetest.SeedRoster(t, repos, "eng-manchester-city", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Position: "F S",
			Apps: 35, Minutes: 2979, Goals: 27, Assists: 8, XG: 28.8, XA: 5.5}})

	storagetest.SeedCompetition(t, repos, storagetest.ChampionsLeague(), "2025", city)
	storagetest.SeedRoster(t, repos, "eng-manchester-city", "uefa-champions-league", "uefa-champions-league:2025",
		[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Position: "F S",
			Apps: 11, Minutes: 900, Goals: 9, Assists: 2, XG: 8.1, XA: 1.4}})

	return repos
}

func TestCompetitionsAppearAsSeparateTabs(t *testing.T) {
	out := settled(t, newDeps(seedTwoCompetitions(t)), "").View()

	for _, want := range []string{"Premier League", "Champions League", "Erling Haaland"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q\n---\n%s", want, out)
		}
	}
}

// The same rule as the Team screen: a striker's league and cup returns are
// different numbers and summing them misrepresents both.
func TestCareerTotalsAreNotBlendedAcrossCompetitions(t *testing.T) {
	s := settled(t, newDeps(seedTwoCompetitions(t)), "")

	m := s.(*player.Model)
	league := m.CareerInCurrentCompetition()
	if league.Goals != 27 || league.Appearances != 35 {
		t.Errorf("league career = %+v, want 27 goals in 35 apps", league)
	}
	// 27 + 9 = 36 would mean the two competitions were summed together.
	if league.Goals == 36 {
		t.Fatal("career totals blended the cup into the league")
	}

	s = tuitest.Key(t, s, "right")
	cup := s.(*player.Model).CareerInCurrentCompetition()
	if cup.Goals != 9 || cup.Appearances != 11 {
		t.Errorf("cup career = %+v, want 9 goals in 11 apps", cup)
	}
}

func TestCareerLineNamesTheCompetitionItTotals(t *testing.T) {
	out := settled(t, newDeps(seedTwoCompetitions(t)), "").View()

	if !strings.Contains(out, "Career in Premier League") {
		t.Errorf("career line must name its competition so it can't be read as a career total\n---\n%s", out)
	}
	if !strings.Contains(out, "27 goals") {
		t.Errorf("career line missing totals\n---\n%s", out)
	}
}

func TestPer90RatesAccompanyRawTotals(t *testing.T) {
	out := settled(t, newDeps(seedTwoCompetitions(t)), "").View()

	// 27 goals in 2979 minutes is 0.82/90.
	if !strings.Contains(out, "0.82/90") {
		t.Errorf("expected a per-90 goal rate\n---\n%s", out)
	}
}

// Under 90 minutes a single goal implies an absurd rate; that's noise, not
// insight, so the rate is suppressed.
func TestPer90IsSuppressedForTinySampleSizes(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025",
		[]storagetest.TeamRow{{ID: "eng-arsenal", Name: "Arsenal", Position: 1, MP: 38, Pts: 85}})
	storagetest.SeedRoster(t, repos, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Apps: 1, Minutes: 12, Goals: 1}})

	out := settled(t, newDeps(repos), "").View()
	if strings.Contains(out, "/90") {
		t.Errorf("a 12-minute cameo should not produce a per-90 rate\n---\n%s", out)
	}
}

func TestSelectedSeasonShowsFinishingAgainstExpectedGoals(t *testing.T) {
	out := settled(t, newDeps(seedTwoCompetitions(t)), "").View()

	// 27 goals from 28.8 xG is 1.8 fewer than expected.
	if !strings.Contains(out, "-1.8 vs G") {
		t.Errorf("expected a goals-vs-xG comparison\n---\n%s", out)
	}
}

func TestOpensOnACompetitionThePlayerPlayedForTheGivenClub(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "eng-manchester-city")

	m := s.(*player.Model)
	if _, ok := m.CurrentCompetition(); !ok {
		t.Fatal("no competition selected")
	}
	if len(m.CurrentRows()) == 0 {
		t.Error("expected the opening tab to have rows")
	}
}

func TestEnterOpensTheClubForTheSelectedSeason(t *testing.T) {
	var openedTeam models.Team
	var openedComp, openedSeason string

	deps := newDeps(seedTwoCompetitions(t))
	deps.Nav.Team = func(_ core.Deps, tm models.Team, competitionID, seasonID string) core.Screen {
		openedTeam, openedComp, openedSeason = tm, competitionID, seasonID
		return stubScreen{}
	}

	s := settled(t, deps, "")
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("expected enter to push the Team screen")
	}
	if openedTeam.Name != "Manchester City" {
		t.Errorf("opened %q", openedTeam.Name)
	}
	if openedComp != "eng-premier-league" || openedSeason != "eng-premier-league:2025" {
		t.Errorf("opened %s / %s, want the selected row's competition and season", openedComp, openedSeason)
	}
}

func TestEmptyStateWhenPlayerHasNoCachedSeasons(t *testing.T) {
	out := settled(t, newDeps(storagetest.New(t)), "").View()

	if !strings.Contains(out, "No cached seasons") {
		t.Errorf("expected an empty state\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("empty state must name the command that fixes it\n---\n%s", out)
	}
}

// A player whose provider supplied no expected-goals data must show the
// missing marker, not a career xG of 0.0.
func TestCareerWithoutExpectedGoalsOmitsThoseTotals(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025",
		[]storagetest.TeamRow{{ID: "eng-arsenal", Name: "Arsenal", Position: 1, MP: 38, Pts: 85}})

	ctx := context.Background()
	if err := repos.UpsertPlayer(ctx, haaland); err != nil {
		t.Fatalf("UpsertPlayer: %v", err)
	}
	if err := repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
		PlayerID: haaland.ID, TeamID: "eng-arsenal",
		CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Appearances: 20, Minutes: 1800, Goals: 10, Assists: 4,
		Source: "test", UpdatedAt: storagetest.Fixed,
	}); err != nil {
		t.Fatalf("UpsertPlayerSeasonStats: %v", err)
	}

	s := settled(t, newDeps(repos), "")
	career := s.(*player.Model).CareerInCurrentCompetition()
	if career.HasXG {
		t.Error("career should not claim expected-goals data it never received")
	}

	out := s.View()
	if strings.Contains(out, "0.0 xG") {
		t.Errorf("absent xG rendered as a zero total\n---\n%s", out)
	}
}

// A player's history spans clubs, and two clubs sharing a prefix must stay
// distinguishable at any width.
func TestNarrowTerminalsKeepClubNamesIntact(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: "eng-manchester-city", Name: "Manchester City", Position: 1, MP: 38, Pts: 88},
	})
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2024", []storagetest.TeamRow{
		{ID: "eng-manchester-united", Name: "Manchester United", Position: 2, MP: 38, Pts: 80},
	})
	storagetest.SeedRoster(t, repos, "eng-manchester-city", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Apps: 35, Minutes: 2979, Goals: 27, XG: 28.8}})
	storagetest.SeedRoster(t, repos, "eng-manchester-united", "eng-premier-league", "eng-premier-league:2024",
		[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Apps: 30, Minutes: 2500, Goals: 20, XG: 21.4}})

	for _, width := range []int{60, 70, 80, 100, 130} {
		s := tuitest.Settle(t, player.New(newDeps(repos), haaland, ""))
		out := tuitest.Resize(t, s, width, 34).View()

		for _, club := range []string{"Manchester City", "Manchester United"} {
			if !strings.Contains(out, club) {
				t.Errorf("width %d truncated %q\n---\n%s", width, club, out)
			}
		}
	}
}
