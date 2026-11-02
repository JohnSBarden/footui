package team_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

// withLastGame seeds the recent games and g6 (Northbridge 0–2 Arsenal,
// the newest) in full.
func withLastGame(t *testing.T) core.Deps {
	t.Helper()
	repos := seedTwoCompetitions(t)
	seedGames(t, repos)
	if err := repos.PutMatchDetail(context.Background(), storagetest.SampleMatchDetail("g6")); err != nil {
		t.Fatal(err)
	}
	return newDeps(repos)
}

func TestTheLastGameCardShowsTheNewestGameInFull(t *testing.T) {
	s := settled(t, withLastGame(t), "", "")
	out := s.View()
	i := strings.Index(out, "LAST GAME")
	if i < 0 {
		t.Fatalf("no Last game card\n---\n%s", out)
	}
	if j := strings.Index(out, "RECENT"); j < 0 || j > i {
		t.Errorf("the card isn't under the Recent line\n---\n%s", out)
	}
	card := out[i:]
	for _, want := range []string{
		"Premier League", "Northbridge Park", "e details",
		"Northbridge  0–2  Arsenal", "Arsenal: Havertz 16', Saka 54' (pen)",
		"0' ", " 90'", "poss 44%", "56%", "shots 6 · 17", "on target 1 · 5",
	} {
		if !strings.Contains(card, want) {
			t.Errorf("card lacks %q\n---\n%s", want, card)
		}
	}
	if !keysHave(s, "e") {
		t.Error("e isn't in the footer")
	}
}

func TestNoCardWithoutTheDetail(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedGames(t, repos)
	out := settled(t, newDeps(repos), "", "").View()
	if strings.Contains(out, "LAST GAME") {
		t.Errorf("a card with nothing in it\n---\n%s", out)
	}
}

func TestNoCardForAGameTheSourceDidntHave(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedGames(t, repos)
	empty := models.MatchDetail{MatchID: "g6", Final: true, FetchedAt: storagetest.Fixed}
	if err := repos.PutMatchDetail(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	if out := settled(t, newDeps(repos), "", "").View(); strings.Contains(out, "LAST GAME") {
		t.Errorf("an empty card\n---\n%s", out)
	}
}

func TestTheCardGoesWhenTheScreenIsShort(t *testing.T) {
	s := tuitest.Resize(t, settled(t, withLastGame(t), "", ""), 110, 24)
	out := s.View()
	if strings.Contains(out, "LAST GAME") {
		t.Errorf("the card squeezed the tables\n---\n%s", out)
	}
	if !strings.Contains(out, "Seasons") {
		t.Errorf("the tables are gone\n---\n%s", out)
	}
}

func TestEOpensTheLastGame(t *testing.T) {
	deps := withLastGame(t)
	var opened models.Match
	var from string
	deps.Nav.Match = func(_ core.Deps, m models.Match, teamID string) core.Screen {
		opened, from = m, teamID
		return tuitest.Stub{}
	}
	s := settled(t, deps, "", "")

	_, cmd := s.Update(keyRunes("e"))
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("e pushed nothing")
	}
	if opened.ID != "g6" || from != arsenal.ID {
		t.Errorf("opened %q from %q, want g6 from Arsenal", opened.ID, from)
	}

	opened = models.Match{}
	_, cmd = s.Update(core.ClickMsg{ID: "lastgame"})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok || opened.ID != "g6" {
		t.Errorf("a click on the card opened %q", opened.ID)
	}
}

func TestEDoesNothingWithoutGames(t *testing.T) {
	deps := newDeps(seedTwoCompetitions(t))
	deps.Nav.Match = func(core.Deps, models.Match, string) core.Screen {
		t.Error("opened a game the club hasn't played")
		return tuitest.Stub{}
	}
	s := settled(t, deps, "", "")
	if _, cmd := s.Update(keyRunes("e")); cmd != nil {
		tuitest.CapturedCmd(cmd)
	}
	if keysHave(s, "e") {
		t.Error("e is offered with no game to open")
	}
	if _, ok := s.(*team.Model).LastGame(); ok {
		t.Error("a last game with no games")
	}
}

func keyRunes(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }

func keysHave(s core.Screen, k string) bool {
	for _, b := range s.Keys().Short {
		if b.Help().Key == k {
			return true
		}
	}
	return false
}
