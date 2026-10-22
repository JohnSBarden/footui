package team_test

import (
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func seedSeasonsAndSquad(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	for i, year := range []string{"2023", "2024", "2025"} {
		id := storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), year, []storagetest.TeamRow{
			{ID: arsenal.ID, Name: "Arsenal", Position: 3 - i, MP: 38, W: 20 + i, D: 8, L: 10 - i, GF: 70, GA: 30, Pts: 68 + 3*i, XG: 70, XGA: 32},
		})
		storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", id, storagetest.SampleSquad())
	}
	return repos
}

func sized(t *testing.T, repos storage.Repositories, w, h int) core.Screen {
	t.Helper()
	s := tuitest.Settle(t, team.New(newDeps(repos), arsenal, "eng-premier-league", "eng-premier-league:2025"))
	return tuitest.Resize(t, s, w, h)
}

// The screen must never exceed the lines it's given.
func TestTeamFitsTheTerminal(t *testing.T) {
	repos := seedSeasonsAndSquad(t)
	for _, size := range [][2]int{{80, 19}, {100, 25}, {120, 35}, {200, 50}} {
		out := sized(t, repos, size[0], size[1]).View()
		if got := strings.Count(out, "\n") + 1; got > size[1] {
			t.Errorf("%dx%d: drew %d lines\n---\n%s", size[0], size[1], got, out)
		}
		if !strings.Contains(out, "2025-26") {
			t.Errorf("%dx%d: the seasons table is missing\n---\n%s", size[0], size[1], out)
		}
	}
}

// Too short for both tables, the screen shows the focused one and names
// the other, and tab swaps them.
func TestShortTerminalShowsOneTableAtATime(t *testing.T) {
	s := sized(t, seedSeasonsAndSquad(t), 80, 16)
	out := s.View()
	if !strings.Contains(out, "tab switch") || strings.Contains(out, "Bukayo Saka") {
		t.Fatalf("want the seasons alone, with the squad named\n---\n%s", out)
	}
	out = tuitest.Key(t, s, "tab").View()
	if !strings.Contains(out, "Bukayo Saka") || strings.Contains(out, "2023-24") {
		t.Errorf("tab should show the squad in place of the seasons\n---\n%s", out)
	}
}

func TestWideTerminalSetsTheSquadBesideTheSeasons(t *testing.T) {
	out := sized(t, seedSeasonsAndSquad(t), 200, 50).View()
	beside := false
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Season") && strings.Contains(line, "Player") {
			beside = true
		}
	}
	if !beside {
		t.Errorf("seasons and squad headers should share a line\n---\n%s", out)
	}
	if !strings.Contains(out, "xGA") {
		t.Errorf("the seasons column lost its expected-goals columns\n---\n%s", out)
	}
}
