package browse_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/browse"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func newDeps(repos storage.Repositories) core.Deps {
	return core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
}

func seeded(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: "eng-manchester-united", Name: "Manchester United", Position: 1, MP: 38, Pts: 80, GF: 70, GA: 40},
		{ID: "eng-manchester-city", Name: "Manchester City", Position: 2, MP: 38, Pts: 78, GF: 75, GA: 35},
		{ID: "eng-newcastle-united", Name: "Newcastle United", Position: 3, MP: 38, Pts: 60, GF: 55, GA: 50},
	})
	storagetest.SeedRoster(t, repos, "eng-manchester-city", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{{ID: "p-haaland", Name: "Erling Haaland", Position: "F S", Apps: 35, Minutes: 2979, Goals: 27, Assists: 8, XG: 28.8, XA: 5.5}})
	return repos
}

func open(t *testing.T, deps core.Deps, query string) core.Screen {
	t.Helper()
	return tuitest.Resize(t, tuitest.Settle(t, browse.New(deps, query)), 100, 30)
}

func TestEmptyQueryListsCachedCompetitions(t *testing.T) {
	out := open(t, newDeps(seeded(t)), "").View()

	if !strings.Contains(out, "Premier League") {
		t.Errorf("empty query should list cached competitions\n---\n%s", out)
	}
	if !strings.Contains(out, "competition") {
		t.Errorf("results should be labelled by type\n---\n%s", out)
	}
}

func TestSearchFindsTeamsAndPlayers(t *testing.T) {
	out := open(t, newDeps(seeded(t)), "manchester").View()
	if !strings.Contains(out, "Manchester United") || !strings.Contains(out, "Manchester City") {
		t.Errorf("expected both Manchester clubs\n---\n%s", out)
	}

	out = open(t, newDeps(seeded(t)), "haaland").View()
	if !strings.Contains(out, "Erling Haaland") {
		t.Errorf("expected the player match\n---\n%s", out)
	}
	if !strings.Contains(out, "player") {
		t.Errorf("player results should be labelled as such\n---\n%s", out)
	}
}

func TestExactMatchRanksAboveOtherSubstringMatches(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "Manchester United")

	m := s.(*browse.Model)
	first, ok := m.SelectedResult()
	if !ok {
		t.Fatal("no results")
	}
	if first.Name() != "Manchester United" {
		t.Errorf("first result is %q, want the exact match ranked first", first.Name())
	}
}

func TestWordPrefixRanksAboveMidWordMatch(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "united")

	m := s.(*browse.Model)
	first, _ := m.SelectedResult()
	// Both clubs contain "united"; neither starts with it, so the tie
	// breaks alphabetically among equal word-prefix matches.
	if !strings.Contains(first.Name(), "United") {
		t.Errorf("first result %q should be one of the United clubs", first.Name())
	}
}

func TestTypingNarrowsResults(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "")
	if out := s.View(); !strings.Contains(out, "Premier League") {
		t.Fatalf("expected the competition list first\n---\n%s", out)
	}

	for _, r := range "haaland" {
		updated, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		s = tuitest.Drive(t, updated, cmd)
	}

	out := s.View()
	if !strings.Contains(out, "Erling Haaland") {
		t.Errorf("typing should have narrowed to the player\n---\n%s", out)
	}
	if strings.Contains(out, "Premier League") {
		t.Errorf("stale competition list still shown after typing\n---\n%s", out)
	}
}

func TestEnterOpensACompetition(t *testing.T) {
	var opened string
	deps := newDeps(seeded(t))
	deps.Nav.League = func(_ core.Deps, id, _ string) core.Screen { opened = id; return stubScreen{} }

	s := open(t, deps, "premier")
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("expected enter to push a screen")
	}
	if opened != "eng-premier-league" {
		t.Errorf("opened %q", opened)
	}
}

func TestEnterOpensATeam(t *testing.T) {
	var opened models.Team
	deps := newDeps(seeded(t))
	deps.Nav.Team = func(_ core.Deps, tm models.Team, _, _ string) core.Screen { opened = tm; return stubScreen{} }

	s := open(t, deps, "Manchester City")
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("expected enter to push a screen")
	}
	if opened.Name != "Manchester City" {
		t.Errorf("opened %q", opened.Name)
	}
}

// The search box must keep bare letters, or typing a club with a "q" in it
// would quit the program.
func TestSearchBoxCapturesTextSoGlobalShortcutsDoNotFire(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "")

	entry, ok := s.(core.TextEntry)
	if !ok {
		t.Fatal("Browse should implement core.TextEntry")
	}
	if !entry.CapturingText() {
		t.Error("the focused search box should be capturing text")
	}
}

func TestFirstRunEmptyStateExplainsHowToGetData(t *testing.T) {
	out := open(t, newDeps(storagetest.New(t)), "").View()

	if !strings.Contains(out, "Nothing cached yet") {
		t.Errorf("expected a first-run empty state\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("empty state must name the command that fixes it\n---\n%s", out)
	}
}

func TestNoMatchEmptyStateIsDistinctFromFirstRun(t *testing.T) {
	out := open(t, newDeps(seeded(t)), "zzzzz").View()

	if !strings.Contains(out, "No cached competition, team, or player matches") {
		t.Errorf("expected a no-match empty state\n---\n%s", out)
	}
	if strings.Contains(out, "Nothing cached yet") {
		t.Errorf("a failed search must not claim the cache is empty\n---\n%s", out)
	}
}

// Searching from an ASCII keyboard must find accented names — the common
// case for a tool full of them.
func TestBrowseFindsAccentedNamesFromASCIIQueries(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: "eng-arsenal", Name: "Arsenal", Position: 1, MP: 38, Pts: 85},
	})
	storagetest.SeedRoster(t, repos, "eng-arsenal", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{
			{ID: "p-zub", Name: "Martín Zubimendi", Position: "D M S", Apps: 38, Minutes: 3040},
			{ID: "p-tim", Name: "Jurriën Timber", Position: "D S", Apps: 30, Minutes: 2475},
		})

	for query, want := range map[string]string{
		"martin":  "Martín Zubimendi",
		"jurrien": "Jurriën Timber",
		"Martín":  "Martín Zubimendi",
	} {
		out := open(t, newDeps(repos), query).View()
		if !strings.Contains(out, want) {
			t.Errorf("searching %q should find %q\n---\n%s", query, want, out)
		}
	}
}

func TestFirstRunEmptyStateOffersTheRefreshKey(t *testing.T) {
	out := open(t, newDeps(storagetest.New(t)), "").View()
	// The search box owns "r", so ctrl+r is the key offered here.
	if !strings.Contains(out, "ctrl+r") {
		t.Errorf("first-run empty state should offer the in-app refresh key\n---\n%s", out)
	}
}

// With nothing cached there is nothing to pick from, so the refresh target
// is the default competition the empty state names.
func TestRefreshTargetOnFirstRunIsTheDefaultCompetition(t *testing.T) {
	s := open(t, newDeps(storagetest.New(t)), "")
	got, ok := s.(core.Refreshable).RefreshTarget()
	if !ok || got != "eng-premier-league" {
		t.Errorf("RefreshTarget = %q, %v; want the default competition", got, ok)
	}
}

func TestRefreshTargetIsTheSelectedCompetitionResult(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "premier league")
	got, ok := s.(core.Refreshable).RefreshTarget()
	if !ok || got != "eng-premier-league" {
		t.Errorf("RefreshTarget = %q, %v; want the highlighted competition", got, ok)
	}
}

func TestRefreshTargetIsAbsentWhenATeamIsSelected(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "newcastle")
	if got, ok := s.(core.Refreshable).RefreshTarget(); ok {
		t.Errorf("RefreshTarget = %q; a team result has no competition to refresh", got)
	}
}

func TestRefreshedMsgRerunsTheSearch(t *testing.T) {
	repos := storagetest.New(t)
	s := open(t, newDeps(repos), "")
	if !strings.Contains(s.View(), "Nothing cached yet") {
		t.Fatalf("precondition: expected the first-run state\n---\n%s", s.View())
	}

	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", nil)
	updated, cmd := s.Update(core.RefreshedMsg{CompetitionID: "eng-premier-league"})
	s = tuitest.Drive(t, updated, cmd)

	if !strings.Contains(s.View(), "Premier League") {
		t.Errorf("the newly cached competition should be listed\n---\n%s", s.View())
	}
}

// tab hands the keyboard to the results, where bare letters are
// shortcuts again — otherwise q, ? and r could never be pressed on the
// screen footy opens on.
func TestTabMovesFocusToTheResults(t *testing.T) {
	s := tuitest.Key(t, open(t, newDeps(seeded(t)), "manchester"), "tab")

	if s.(core.TextEntry).CapturingText() {
		t.Fatal("after tab the search box should have let go of the keyboard")
	}
	before, _ := s.(*browse.Model).SelectedResult()
	s = tuitest.Key(t, s, "j")
	after, _ := s.(*browse.Model).SelectedResult()
	if before.Name() == after.Name() {
		t.Error("j should move down the results once they have focus")
	}

	s = tuitest.Key(t, s, "/")
	if !s.(core.TextEntry).CapturingText() {
		t.Error("/ should return focus to the search box")
	}
}

func TestEscClearsTheQueryBeforeLeaving(t *testing.T) {
	s := open(t, newDeps(seeded(t)), "manchester")
	esc := s.(core.Escaper)

	handled, cmd := esc.Escape()
	if !handled {
		t.Fatal("esc with a query should clear it, not leave the screen")
	}
	s = tuitest.Drive(t, s, cmd)
	if !strings.Contains(s.View(), "Premier League") {
		t.Errorf("clearing the query should list competitions again\n---\n%s", s.View())
	}

	if handled, _ := s.(core.Escaper).Escape(); handled {
		t.Error("esc on an empty search box should go back")
	}
}

func TestEscFromTheResultsReturnsToTheSearchBox(t *testing.T) {
	s := tuitest.Key(t, open(t, newDeps(seeded(t)), "manchester"), "tab")

	if handled, _ := s.(core.Escaper).Escape(); !handled {
		t.Fatal("esc from the results should return to the box")
	}
	if !s.(core.TextEntry).CapturingText() {
		t.Error("the search box should have focus again")
	}
}
