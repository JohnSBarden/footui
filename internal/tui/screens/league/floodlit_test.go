package league_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

// twentyClubs fills every zone. Club 7 has mid-table points and top-four
// chances.
func twentyClubs() []storagetest.TeamRow {
	rows := make([]storagetest.TeamRow, 20)
	for i := range rows {
		pos := i + 1
		rows[i] = storagetest.TeamRow{
			ID: fmt.Sprintf("eng-club-%02d", pos), Name: fmt.Sprintf("Club %02d", pos), Position: pos,
			MP: 6, Pts: 20 - pos, GF: 12 - pos/2, GA: 4 + pos/2, XG: 10 - float64(pos)/4, XGA: 6 + float64(pos)/5,
		}
	}
	return rows
}

func seedXPts(t *testing.T, repos storage.Repositories, seasonID string, xpts func(pos int) float64) {
	t.Helper()
	ctx := context.Background()
	stats, err := repos.Standings(ctx, "eng-premier-league", seasonID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stats {
		s.Extras = map[string]float64{models.StatExpectedPoints: xpts(*s.Position)}
		if err := repos.UpsertTeamSeasonStats(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
}

func leagueView(t *testing.T, repos storage.Repositories, th styles.Theme, width int) string {
	t.Helper()
	deps := core.Deps{Ctx: context.Background(), Repos: repos, Theme: th}
	s := tuitest.Settle(t, league.New(deps, "eng-premier-league"))
	return tuitest.Resize(t, s, width, 40).View()
}

func TestLeagueZonesHaveLettersWithoutColor(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", twentyClubs())
	view := leagueView(t, repos, styles.Plain(), 140)
	for _, want := range []string{"C Champions L.", "E Europa L.", "↓ relegation", "C   1 ● Club 01", "E   5 ● Club 05", "↓  20 ● Club 20"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q\n---\n%s", want, view)
		}
	}
}

func TestLeagueInColourHasGutterDotsAndBars(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", twentyClubs())
	view := ansi.Strip(leagueView(t, repos, styles.Default(), 140))
	for _, want := range []string{"▌", "●", "Champions L.", "█", "┃", "TEAM"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q\n---\n%s", want, view)
		}
	}
}

func TestLeagueInsightNamesTheBiggestGapToXPts(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", twentyClubs())
	seedXPts(t, repos, "eng-premier-league:2025", func(pos int) float64 {
		if pos == 7 {
			return 21.5 // 13 pts from 21.5 xPts: owed 8.5
		}
		return float64(20-pos) + 0.3
	})
	view := leagueView(t, repos, styles.Plain(), 140)
	if !strings.Contains(view, "Club 07: 13 pts from 21.5 xPts · biggest gap in the table") {
		t.Errorf("insight line\n---\n%s", view)
	}
}

func TestLeagueWithoutXPtsHasNoInsight(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", twentyClubs())
	if strings.Contains(leagueView(t, repos, styles.Plain(), 140), "biggest gap") {
		t.Error("no xPts, no insight")
	}
}

func TestLeagueWithoutARuleHasNoZones(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2019", twentyClubs())
	if strings.Contains(leagueView(t, repos, styles.Plain(), 140), "Champions L.") {
		t.Error("2019-20 predates the seeded rule: no zones")
	}
}

func TestLeagueFitsEightyColumns(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", twentyClubs())
	for _, th := range []styles.Theme{styles.Plain(), styles.Default()} {
		for _, l := range strings.Split(leagueView(t, repos, th, 80), "\n") {
			if lipgloss.Width(l) > 80 {
				t.Errorf("overflow %d: %q", lipgloss.Width(l), ansi.Strip(l))
			}
		}
	}
}
