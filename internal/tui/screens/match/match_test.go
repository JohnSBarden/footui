package match_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/match"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

const (
	home = "eng-northbridge"
	away = "eng-arsenal"
)

// seed stores Northbridge 0–2 Arsenal and, with full, its detail.
func seed(t *testing.T, full bool) (core.Deps, models.Match) {
	t.Helper()
	ctx := context.Background()
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: home, Name: "Northbridge", Position: 9},
		{ID: away, Name: "Arsenal", Position: 1},
	})
	two, zero := 2, 0
	g := models.Match{
		ID: "g1", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Kickoff: time.Date(2026, 3, 18, 20, 0, 0, 0, time.UTC), Status: models.MatchFullTime,
		HomeTeamID: home, AwayTeamID: away, HomeScore: &zero, AwayScore: &two,
		Source: "test", UpdatedAt: storagetest.Fixed,
	}
	if err := repos.UpsertMatches(ctx, []models.Match{g}); err != nil {
		t.Fatal(err)
	}
	if full {
		if err := repos.PutMatchDetail(ctx, storagetest.SampleMatchDetail(g.ID)); err != nil {
			t.Fatal(err)
		}
	}
	return core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain()}, g
}

func open(t *testing.T, deps core.Deps, g models.Match, teamID string) core.Screen {
	t.Helper()
	return tuitest.Resize(t, tuitest.Settle(t, match.New(deps, g, teamID)), 110, 40)
}

func contains(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q\n---\n%s", w, out)
		}
	}
}

func TestTheHeaderHasTheScoreScorersAndVenue(t *testing.T) {
	deps, g := seed(t, true)
	out := open(t, deps, g, away).View()
	contains(t, out, "Northbridge  0–2  Arsenal", "Premier League", "Wed 18 Mar 2026",
		"Northbridge Park", "41,250", "● Arsenal: Havertz 16', Saka 54' (pen)", "0' ", " 90'",
		"1 Timeline", "2 Stats", "3 Lineups", "4 Commentary")
}

func TestTheTitleNamesTheGame(t *testing.T) {
	deps, g := seed(t, true)
	if got := open(t, deps, g, away).Title(); got != "Northbridge v Arsenal" {
		t.Errorf("title %q", got)
	}
}

// Each team's name sits over its end of the bar.
func TestStatsNamesSitOverTheirNumbers(t *testing.T) {
	deps, g := seed(t, true)
	lines := strings.Split(tuitest.Key(t, open(t, deps, g, away), "2").View(), "\n")
	var head, poss string
	for i, l := range lines {
		if strings.Contains(l, "Possession") {
			head, poss = lines[i-1], l
		}
	}
	bar := []rune(poss)
	start, end := strings.IndexRune(string(bar), '#'), strings.LastIndex(string(bar), ".")+1
	if strings.Index(head, "Northbridge") != start {
		t.Errorf("home name not over the bar's left end\n%s\n%s", head, poss)
	}
	if strings.Index(head, "Arsenal")+len("Arsenal") != end {
		t.Errorf("away name not over the bar's right end\n%s\n%s", head, poss)
	}
}

func TestTheTimelineKeepsTheRunningScore(t *testing.T) {
	deps, g := seed(t, true)
	out := open(t, deps, g, away).View()
	contains(t, out, "Havertz (Saka)  0–1", "Saka (pen)  0–2", "Half time 0–1", "Full time 0–2",
		"Bench for Defender", "16'", "81'")
	// Home events sit left of the clock, away events right of it.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Defender") && strings.Contains(line, "33'") &&
			strings.Index(line, "Defender") > strings.Index(line, "33'") {
			t.Errorf("a home booking on the away side: %q", line)
		}
		if strings.Contains(line, "Havertz (Saka)") && strings.Index(line, "Havertz") < strings.Index(line, "16'") {
			t.Errorf("an away goal on the home side: %q", line)
		}
	}
}

func TestTabsSwitchSections(t *testing.T) {
	deps, g := seed(t, true)
	s := open(t, deps, g, away)

	s = tuitest.Key(t, s, "2")
	if s.(*match.Model).Tab() != match.TabStats {
		t.Fatal("2 didn't open Stats")
	}
	contains(t, s.View(), "Possession", "44%", "56%", "Shots", "17", "Pass accuracy", "79%", "90%", "Corners")
	if strings.Contains(s.View(), "Interceptions") {
		t.Error("a stat neither side has is shown")
	}

	s = tuitest.Key(t, s, "]")
	contains(t, s.View(), "4-3-3", "4-2-3-1", "Tom Keeper", "Kai Havertz", "SUBS", "▲70'", "▼70'", "Unused Ulf")

	s = tuitest.Key(t, s, "tab")
	contains(t, s.View(), "Lineups are announced", "Goal! Northbridge 0, Arsenal 1", "54'")

	s = tuitest.Key(t, s, "tab")
	if s.(*match.Model).Tab() != match.TabTimeline {
		t.Error("tab doesn't wrap round")
	}
	s = tuitest.Key(t, s, "[")
	if s.(*match.Model).Tab() != match.TabCommentary {
		t.Error("[ doesn't step back")
	}
	s, _ = s.Update(core.ClickMsg{ID: "tab:1"})
	if s.(*match.Model).Tab() != match.TabStats {
		t.Error("a click on a tab didn't open it")
	}
}

func TestLongSectionsScroll(t *testing.T) {
	deps, g := seed(t, true)
	s := tuitest.Resize(t, open(t, deps, g, away), 60, 11)
	s = tuitest.Key(t, s, "4")
	if !strings.Contains(s.View(), "more") {
		t.Fatalf("no sign there's more\n---\n%s", s.View())
	}
	s = tuitest.Key(t, s, "end")
	if out := s.View(); strings.Contains(out, "more") || !strings.Contains(out, "penalty") {
		t.Errorf("end didn't reach the last line\n---\n%s", out)
	}
	s = tuitest.Key(t, s, "home")
	contains(t, s.View(), "Lineups are announced")
}

func TestWithoutTheDetailItSaysHowToFetchIt(t *testing.T) {
	deps, g := seed(t, false)
	out := open(t, deps, g, away).View()
	contains(t, out, "Northbridge  0–2  Arsenal", "hasn't been fetched in full", "r")
	if strings.Contains(out, "Timeline") {
		t.Errorf("tabs over nothing\n---\n%s", out)
	}
}

func TestOOpensTheOpponent(t *testing.T) {
	deps, g := seed(t, true)
	var opened models.Team
	deps.Nav.Team = func(_ core.Deps, team models.Team, _, _ string) core.Screen {
		opened = team
		return tuitest.Stub{}
	}
	s := open(t, deps, g, away)
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok || opened.ID != home {
		t.Errorf("o opened %q, want the home side", opened.ID)
	}
}

func TestRefreshIsTheClubItWasOpenedFrom(t *testing.T) {
	deps, g := seed(t, true)
	r, ok := open(t, deps, g, away).(core.RefreshRequester).RefreshRequest()
	if !ok || r.TeamID != away || r.Name != "Arsenal" {
		t.Errorf("refresh %+v, %v", r, ok)
	}
	if _, ok := open(t, deps, g, "").(core.RefreshRequester).RefreshRequest(); ok {
		t.Error("a refresh with no club")
	}
}
