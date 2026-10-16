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
	for _, want := range []string{"FWD", "Manchester City", "GOALS vs xG", "27 / 28.8", "SEASON BY SEASON", "beat the expected number"} {
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

// understat lists every position a player has played, alphabetically, so
// "F M S" is no claim to be a forward. The badge names them all, and the
// chart follows what the player produces: Bruno-style, more assists than
// goals, gets the assists chart.
func TestACreatorListedAsFMSGetsTheAssistsChart(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())
	storagetest.SeedRoster(t, repos, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{{ID: "p-bruno", Name: "Bruno Fernandes", Position: "F M S", Apps: 35, Minutes: 3082, Goals: 9, Assists: 21, XG: 11.9, XA: 17.8}})
	bruno := models.Player{ID: "p-bruno", Name: "Bruno Fernandes", Position: "F M S", Gender: "men"}
	out := tuitest.Resize(t, tuitest.Settle(t, player.New(newDeps(repos), bruno, "")), 112, 34).View()
	if !strings.Contains(out, "ASSISTS vs xA") || !strings.Contains(out, "FWD·MID") {
		t.Errorf("creator's band\n---\n%s", out)
	}
}

func TestPlayerChartRowsLineUp(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2024", []storagetest.TeamRow{{ID: "eng-manchester-city", Name: "Manchester City", Position: 1, MP: 38}})
	storagetest.SeedRoster(t, repos, "eng-manchester-city", "eng-premier-league", "eng-premier-league:2024",
		[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Position: "F S", Apps: 31, Minutes: 2500, Goals: 9, Assists: 2, XG: 12.25, XA: 3}})
	out := settled(t, newDeps(repos), "").View()
	rows := 0
	starts := map[int]bool{}
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, "20"); i >= 0 && strings.Contains(l, " / ") {
			rows++
			starts[lipgloss.Width(l[:i])] = true
		}
	}
	if rows < 2 || len(starts) != 1 {
		t.Errorf("chart rows should start in one column: %v\n---\n%s", starts, out)
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
