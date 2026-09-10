package team_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var arsenal = models.Team{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}

func newDeps(repos storage.Repositories) core.Deps {
	return core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
}

func settled(t *testing.T, deps core.Deps, competitionID, seasonID string) core.Screen {
	t.Helper()
	s := tuitest.Settle(t, team.New(deps, arsenal, competitionID, seasonID))
	return tuitest.Resize(t, s, 110, 44)
}

// seedTwoCompetitions gives Arsenal the same 2025 season in two different
// competitions with deliberately different numbers, which is what the
// no-blending assertions rely on.
func seedTwoCompetitions(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)

	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: arsenal.ID, Name: "Arsenal", Position: 1, MP: 38, W: 26, D: 7, L: 5, GF: 71, GA: 27, Pts: 85, XG: 77.5, XGA: 33.1},
	})
	storagetest.SeedCompetition(t, repos, storagetest.ChampionsLeague(), "2025", []storagetest.TeamRow{
		{ID: arsenal.ID, Name: "Arsenal", Position: 3, MP: 13, W: 8, D: 2, L: 3, GF: 24, GA: 11, Pts: 26, XG: 21.4, XGA: 13.9},
	})
	return repos
}

func TestTeamStatsAreNotBlendedAcrossCompetitions(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025")

	league := s.View()
	// The Premier League tab must show only the league's 38 games and 85
	// points, never 51 games or 111 points.
	if !strings.Contains(league, "85") || !strings.Contains(league, "38") {
		t.Errorf("Premier League tab missing its own figures\n---\n%s", league)
	}
	if strings.Contains(league, "111") || strings.Contains(league, "51 played") {
		t.Errorf("figures were blended across competitions\n---\n%s", league)
	}

	// Switching tabs must show the other competition's distinct numbers.
	s = tuitest.Key(t, s, "right")
	cup := s.View()
	if !strings.Contains(cup, "26") || !strings.Contains(cup, "13 played") {
		t.Errorf("Champions League tab missing its own figures\n---\n%s", cup)
	}
	if strings.Contains(cup, "85") {
		t.Errorf("Premier League points leaked into the Champions League tab\n---\n%s", cup)
	}
}

func TestBothCompetitionsAppearAsTabs(t *testing.T) {
	repos := seedTwoCompetitions(t)
	out := settled(t, newDeps(repos), "", "").View()

	for _, want := range []string{"Premier League", "Champions League"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab bar missing %q\n---\n%s", want, out)
		}
	}
}

func TestOpensOnTheCompetitionAndSeasonDrilledInFrom(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "uefa-champions-league", "uefa-champions-league:2025")

	m := s.(*team.Model)
	comp, ok := m.CurrentCompetition()
	if !ok {
		t.Fatal("no competition selected")
	}
	if comp.ID != "uefa-champions-league" {
		t.Errorf("opened on %q, want the competition drilled in from", comp.ID)
	}
}

func TestDefaultsToFirstCompetitionWhenOpenedWithoutContext(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "", "")

	m := s.(*team.Model)
	comp, ok := m.CurrentCompetition()
	if !ok {
		t.Fatal("no competition selected")
	}
	// The domestic league leads: a continental cup has no tier, and
	// unknown tiers sort after real ones.
	if comp.Name != "Premier League" {
		t.Errorf("defaulted to %q, want the domestic league as the first tab", comp.Name)
	}
}

func TestSquadLoadsForTheSelectedSeason(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	for _, want := range []string{"Squad", "Bukayo Saka", "Declan Rice", "David Raya"} {
		if !strings.Contains(out, want) {
			t.Errorf("squad table missing %q\n---\n%s", want, out)
		}
	}
}

func TestSwitchingCompetitionSwapsTheSquadRatherThanKeepingAStaleOne(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())
	storagetest.SeedRoster(t, repos, arsenal.ID, "uefa-champions-league", "uefa-champions-league:2025", []storagetest.PlayerRow{
		{ID: "p-havertz", Name: "Kai Havertz", Position: "F S", Apps: 11, Minutes: 800, Goals: 5, Assists: 1, XG: 4.2, XA: 1.1},
	})

	s := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025")
	if out := s.View(); !strings.Contains(out, "Bukayo Saka") {
		t.Fatalf("expected the league squad first\n---\n%s", out)
	}

	s = tuitest.Key(t, s, "right")
	out := s.View()
	if !strings.Contains(out, "Kai Havertz") {
		t.Errorf("expected the cup squad after switching tabs\n---\n%s", out)
	}
	if strings.Contains(out, "Bukayo Saka") {
		t.Errorf("stale squad from the previous competition still shown\n---\n%s", out)
	}
}

func TestTabMovesFocusToSquadAndEnterOpensThePlayer(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())

	var opened models.Player
	deps := newDeps(repos)
	deps.Nav.Player = func(_ core.Deps, p models.Player, _ string) core.Screen {
		opened = p
		return stubScreen{}
	}

	s := settled(t, deps, "eng-premier-league", "eng-premier-league:2025")
	s = tuitest.Key(t, s, "tab")
	s = tuitest.Key(t, s, "down") // second squad row

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatalf("expected enter on the squad pane to push the Player screen")
	}
	// The squad is ordered by minutes descending, so row two is Rice.
	if opened.Name != "Declan Rice" {
		t.Errorf("opened %q, want the player under the cursor", opened.Name)
	}
}

func TestTabDoesNotFocusAnEmptySquad(t *testing.T) {
	repos := seedTwoCompetitions(t) // deliberately no roster seeded

	var openedCompetition string
	deps := newDeps(repos)
	deps.Nav.League = func(_ core.Deps, competitionID string) core.Screen {
		openedCompetition = competitionID
		return stubScreen{}
	}

	s := settled(t, deps, "eng-premier-league", "eng-premier-league:2025")
	s = tuitest.Key(t, s, "tab")

	// Focus should have stayed on Seasons, so enter opens the league
	// table rather than a player that doesn't exist.
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("expected enter to push a screen")
	}
	if openedCompetition != "eng-premier-league" {
		t.Errorf("enter opened %q; with no squad, focus must stay on the seasons pane", openedCompetition)
	}
}

func TestEmptyStateWhenTeamHasNoCachedSeasons(t *testing.T) {
	repos := storagetest.New(t)
	out := settled(t, newDeps(repos), "", "").View()

	if !strings.Contains(out, "No cached seasons") {
		t.Errorf("expected an empty state\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("empty state must name the command that fixes it\n---\n%s", out)
	}
}

func TestStatCardsShowExpectedGoalsGap(t *testing.T) {
	repos := seedTwoCompetitions(t)
	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()

	// Arsenal scored 71 against an xG of 77.5, i.e. 6.5 fewer than expected.
	if !strings.Contains(out, "-6.5 vs GF") {
		t.Errorf("expected the goals-vs-xG gap on a stat card\n---\n%s", out)
	}
}
