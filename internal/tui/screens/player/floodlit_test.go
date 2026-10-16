package player_test

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func TestPlayerHeroShowsBadgeClubAndChart(t *testing.T) {
	out := settled(t, newDeps(seedTwoCompetitions(t)), "").View()
	for _, want := range []string{"FORWARD", "Manchester City", "GOALS vs xG", "27 / 28.8", "SEASON BY SEASON", "beat the expected number"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n---\n%s", want, out)
		}
	}
}

func TestPlayerHeroShowsNationalityWhenKnown(t *testing.T) {
	repos := seedTwoCompetitions(t)
	p := haaland
	p.Nationality = "Norway"
	if err := repos.UpsertPlayer(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	s := tuitest.Resize(t, tuitest.Settle(t, player.New(newDeps(repos), p, "")), 112, 34)
	if !strings.Contains(s.View(), "Manchester City · Norway") {
		t.Errorf("nationality missing\n---\n%s", s.View())
	}
}

func TestMidfieldersGetAnAssistsChart(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())
	storagetest.SeedRoster(t, repos, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())
	rice := models.Player{ID: "p-rice", Name: "Declan Rice", Position: "D M S", Gender: "men"}
	s := tuitest.Resize(t, tuitest.Settle(t, player.New(newDeps(repos), rice, "")), 112, 34)
	if !strings.Contains(s.View(), "ASSISTS vs xA") {
		t.Errorf("midfielder chart\n---\n%s", s.View())
	}
}

func TestPlayerWithoutXGHasNoChartOrLegend(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())
	ctx := context.Background()
	p := models.Player{ID: "p-espn", Name: "Some Striker", Position: "F", Gender: "men"}
	if err := repos.UpsertPlayer(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
		PlayerID: p.ID, TeamID: "eng-luton-town", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Appearances: 20, Goals: 6, Assists: 1, Source: "test", UpdatedAt: storagetest.Fixed,
	}); err != nil {
		t.Fatal(err)
	}
	out := tuitest.Resize(t, tuitest.Settle(t, player.New(newDeps(repos), p, "")), 112, 34).View()
	if strings.Contains(out, " vs xG") || strings.Contains(out, "beat the expected") {
		t.Errorf("no-xG player shows xG furniture\n---\n%s", out)
	}
	if !strings.Contains(out, "Some Striker") {
		t.Errorf("player missing\n---\n%s", out)
	}
}

func TestPlayerFitsEightyColumns(t *testing.T) {
	s := tuitest.Resize(t, settled(t, newDeps(seedTwoCompetitions(t)), ""), 80, 24)
	for _, l := range strings.Split(s.View(), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("overflow %d: %q", lipgloss.Width(l), l)
		}
	}
}
