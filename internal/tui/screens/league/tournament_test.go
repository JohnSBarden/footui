package league_test

import (
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func worldCup(t *testing.T, starred ...string) *league.Model {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	deps := newDeps(repos)
	deps.Favorites = core.NewFavorites(nil)
	for _, id := range starred {
		deps.Favorites.Set(models.FavoriteTeam, id, true)
	}
	s := tuitest.Resize(t, tuitest.Settle(t, league.New(deps, "world-cup")), 110, 40)
	return s.(*league.Model)
}

func press(t *testing.T, m *league.Model, keys ...string) *league.Model {
	t.Helper()
	var s core.Screen = m
	for _, k := range keys {
		s = tuitest.Key(t, s, k)
	}
	return s.(*league.Model)
}

// A tournament's table is its groups, one at a time, each ranked within
// itself; [ and ] step through them.
func TestATournamentShowsOneGroupAtATime(t *testing.T) {
	m := worldCup(t)
	out := m.View()
	if m.Group() != "Group K" || !strings.Contains(out, "[K]") || !strings.Contains(out, " L ") {
		t.Errorf("group %q:\n%s", m.Group(), out)
	}
	if !strings.Contains(out, "Carpania") || strings.Contains(out, "Arcadia") {
		t.Errorf("Group K only:\n%s", out)
	}
	m = press(t, m, "]")
	out = m.View()
	if m.Group() != "Group L" || !strings.Contains(out, "Arcadia") || strings.Contains(out, "Carpania") {
		t.Errorf("after ]: %q\n%s", m.Group(), out)
	}
	m = press(t, m, "]")
	if m.Group() != "Group K" {
		t.Error("] wraps around")
	}
	m = press(t, m, "down")
	if row, ok := m.SelectedRow(); !ok || row.Team.ID != "nat-carpania" {
		t.Errorf("selected %+v: the cursor moves within the group", row)
	}
}

func TestATournamentOpensOnAStarredNationsGroup(t *testing.T) {
	if g := worldCup(t, "nat-dravonia").Group(); g != "Group L" {
		t.Errorf("opened on %q, want the starred nation's group", g)
	}
}

// m lists every fixture by stage — the group stage group by group, then
// each knockout round — with shoot-outs and kickoff times.
func TestMatchesViewListsFixturesByRound(t *testing.T) {
	m := worldCup(t)
	m = press(t, m, "m")
	if !m.MatchesView() {
		t.Fatal("m should switch to the fixtures")
	}
	out := m.View()
	gs, qf := strings.Index(out, "GROUP STAGE"), strings.Index(out, "QUARTERFINAL")
	if gs < 0 || qf < gs {
		t.Errorf("stages out of order:\n%s", out)
	}
	if !strings.Contains(out, "1–1 (3–4 p)") || !strings.Contains(out, "4–2") {
		t.Errorf("scores missing:\n%s", out)
	}
	if strings.Contains(out, "[K]") {
		t.Error("the group bar belongs to the tables")
	}
	keysShown := ""
	for _, b := range m.Keys().Short {
		keysShown += b.Help().Key + " "
	}
	if !strings.Contains(keysShown, "m") {
		t.Errorf("footer keys %q should offer m back to the tables", keysShown)
	}
	if m = press(t, m, "m"); m.MatchesView() {
		t.Error("m again goes back to the tables")
	}
}

// Friendlies have no table, so their season opens on the fixtures.
func TestASeasonWithoutATableOpensOnItsFixtures(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	s := tuitest.Settle(t, league.New(newDeps(repos), "int-friendly")).(*league.Model)
	if !s.MatchesView() || !strings.Contains(s.View(), "Arcadia") {
		t.Errorf("friendlies:\n%s", s.View())
	}
}

func TestClubLeaguesHaveNoTournamentKeys(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())
	for _, b := range s.Keys().Short {
		if b.Help().Key == "m" || b.Help().Key == "[/]" {
			t.Errorf("a club league offers %q", b.Help().Key)
		}
	}
	if s.(*league.Model).Group() != "" {
		t.Error("a club table has no groups")
	}
}
