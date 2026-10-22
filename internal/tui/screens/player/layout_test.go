package player_test

import (
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func seedThreeSeasons(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	city := []storagetest.TeamRow{{ID: "eng-manchester-city", Name: "Manchester City", Position: 1, MP: 38, Pts: 88}}
	for i, year := range []string{"2023", "2024", "2025"} {
		id := storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), year, city)
		storagetest.SeedRoster(t, repos, "eng-manchester-city", "eng-premier-league", id,
			[]storagetest.PlayerRow{{ID: haaland.ID, Name: haaland.Name, Position: "F S",
				Apps: 30 + i, Minutes: 2700 + 100*i, Goals: 20 + i, Assists: 5, XG: 22, XA: 4}})
	}
	return repos
}

// A short terminal gives up the stat cards' boxes and the career line's
// second line before it gives up a season.
func TestShortTerminalKeepsEverySeason(t *testing.T) {
	s := tuitest.Settle(t, player.New(newDeps(seedThreeSeasons(t)), haaland, ""))
	for _, size := range [][2]int{{80, 19}, {100, 25}, {200, 50}} {
		out := tuitest.Resize(t, s, size[0], size[1]).View()
		if got := strings.Count(out, "\n") + 1; got > size[1] {
			t.Errorf("%dx%d: drew %d lines\n---\n%s", size[0], size[1], got, out)
		}
		for _, season := range []string{"2023-24", "2024-25", "2025-26"} {
			if !strings.Contains(out, season) {
				t.Errorf("%dx%d: season %s is missing\n---\n%s", size[0], size[1], season, out)
			}
		}
	}
}
