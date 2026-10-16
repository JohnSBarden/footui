package team_test

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func TestTeamHeaderIsAHeroWithItsPosition(t *testing.T) {
	repos := seedTwoCompetitions(t)
	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	for _, want := range []string{"Arsenal", "England · men", "st · 85 pts", "▀█", "RECENT", "[Premier League"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n---\n%s", want, out)
		}
	}
}

func TestSquadHasGoalsAndAssistsBars(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())
	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	if !strings.Contains(out, "G·A") || !strings.Contains(out, "▮") {
		t.Errorf("squad G·A column missing\n---\n%s", out)
	}
}

func TestTeamFitsEightyColumns(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := tuitest.Resize(t, settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025"), 80, 24)
	for _, l := range strings.Split(s.View(), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("overflow %d: %q", lipgloss.Width(l), l)
		}
	}
}

func TestTeamInColourWithAndWithoutClubColours(t *testing.T) {
	repos := seedTwoCompetitions(t)
	deps := newDeps(repos)
	deps.Theme = styles.Default()
	render := func() string {
		s := tuitest.Settle(t, team.New(deps, arsenal, "eng-premier-league", "eng-premier-league:2025"))
		return ansi.Strip(tuitest.Resize(t, s, 110, 44).View())
	}
	if !strings.Contains(render(), "st · 85 pts") {
		t.Error("a club with no colours should still get its band")
	}
	if err := repos.UpsertTeamColors(context.Background(), models.TeamColors{TeamID: arsenal.ID, PrimaryHex: "#EF0107", SecondaryHex: "#FFFFFF", Source: "test"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(render(), "▀▀▀▀") {
		t.Error("the band's rule should be drawn under it")
	}
}
