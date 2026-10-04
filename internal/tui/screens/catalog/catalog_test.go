package catalog

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var laLiga = models.Competition{ID: "esp-la-liga", Name: "La Liga", Country: "Spain", Tier: 1, Gender: "men"}

func newCatalog(t *testing.T) *Model {
	t.Helper()
	repos := storagetest.New(t)
	pl := storagetest.PremierLeague()
	storagetest.SeedCompetition(t, repos, pl, "2025", storagetest.SampleStandings())
	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{
		Provider: "understat", CompetitionID: pl.ID, SeasonID: pl.ID + ":2025",
		StartedAt: storagetest.Fixed, FinishedAt: storagetest.Fixed, Status: models.SyncStatusOK,
	}); err != nil {
		t.Fatal(err)
	}

	deps := core.Deps{
		Ctx:   context.Background(),
		Repos: repos,
		Theme: styles.Plain(),
		Catalog: []core.CatalogEntry{
			{Competition: pl, Seasons: []string{"2026", "2025", "2024"}},
			{Competition: laLiga, Seasons: []string{"2026", "2025", "2024"}},
		},
	}
	m := New(deps)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return tuitest.Settle(t, m).(*Model)
}

func press(m *Model, k string) tea.Cmd {
	var msg tea.KeyMsg
	switch k {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case " ":
		msg = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	_, cmd := m.Update(msg)
	return cmd
}

// requests runs cmd and returns any refresh requests it produced.
func requests(cmd tea.Cmd) []core.RefreshRequest {
	if cmd == nil {
		return nil
	}
	var out []core.RefreshRequest
	var walk func(tea.Msg)
	walk = func(msg tea.Msg) {
		switch msg := msg.(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				if c != nil {
					walk(c())
				}
			}
		case core.RefreshRequestMsg:
			out = append(out, msg.Requests...)
		}
	}
	walk(cmd())
	return out
}

func TestCatalogShowsWhatIsCached(t *testing.T) {
	m := newCatalog(t)
	view := m.View()
	for _, want := range []string{"Premier League", "La Liga", "1 of 3", "2025-26", "cached"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestSelectingAcrossLeaguesQueuesOneRequestPerLeague(t *testing.T) {
	m := newCatalog(t)

	press(m, "a") // every missing Premier League season: 2026, 2024
	press(m, "down")
	press(m, "tab")
	press(m, " ") // La Liga 2026
	if n, l := m.SelectedCount(); n != 3 || l != 2 {
		t.Fatalf("selected %d seasons across %d leagues, want 3 across 2", n, l)
	}

	reqs := requests(press(m, "enter"))
	if len(reqs) != 2 {
		t.Fatalf("got %d requests, want 2: %v", len(reqs), reqs)
	}
	if reqs[0].CompetitionID != "eng-premier-league" || !slices.Equal(reqs[0].Seasons, []string{"2026", "2024"}) {
		t.Errorf("first request %+v", reqs[0])
	}
	if reqs[1].CompetitionID != "esp-la-liga" || !slices.Equal(reqs[1].Seasons, []string{"2026"}) {
		t.Errorf("second request %+v", reqs[1])
	}
	if n, _ := m.SelectedCount(); n != 0 {
		t.Error("selection should clear once queued")
	}
}

func TestEnterWithNothingSelectedDownloadsTheHighlightedSeason(t *testing.T) {
	m := newCatalog(t)
	if reqs := requests(press(m, "enter")); len(reqs) != 0 {
		t.Fatal("enter on the league list should move into its seasons, not download")
	}
	press(m, "down")
	reqs := requests(press(m, "enter"))
	if len(reqs) != 1 || !slices.Equal(reqs[0].Seasons, []string{"2025"}) {
		t.Errorf("got %v, want the highlighted 2025-26", reqs)
	}
}

func TestQueuedAndRunningSeasonsAreMarked(t *testing.T) {
	m := newCatalog(t)
	m.Update(core.RefreshQueueMsg{
		Active:  &core.RefreshRequest{CompetitionID: "eng-premier-league"}, // current season
		Pending: []core.RefreshRequest{{CompetitionID: "eng-premier-league", Seasons: []string{"2024"}}},
	})
	view := m.View()
	if !strings.Contains(view, "downloading") || !strings.Contains(view, "queued") {
		t.Errorf("in-flight seasons should be marked:\n%s", view)
	}
}

func TestRefreshedReloadsCachedState(t *testing.T) {
	m := newCatalog(t)
	comp := laLiga
	storagetest.SeedCompetition(t, m.deps.Repos, comp, "2026", storagetest.SampleStandings())

	_, cmd := m.Update(core.RefreshedMsg{CompetitionID: comp.ID})
	m = tuitest.Drive(t, m, cmd).(*Model)

	if got := m.leagues[1].cachedCount(); got != 1 {
		t.Errorf("La Liga shows %d cached seasons after a refresh, want 1", got)
	}
}

func TestRTargetsTheHighlightedLeague(t *testing.T) {
	m := newCatalog(t)
	press(m, "down")
	if id, ok := m.RefreshTarget(); !ok || id != "esp-la-liga" {
		t.Errorf("refresh target %q, want esp-la-liga", id)
	}
}

func TestFavouriteTargetIsTheHighlightedLeague(t *testing.T) {
	m := newCatalog(t)
	press(m, "down")
	press(m, "tab") // the seasons pane still belongs to La Liga
	want := core.FavoriteTarget{Kind: models.FavoriteCompetition, ID: "esp-la-liga", Name: "La Liga"}
	if got, ok := m.FavoriteTarget(); !ok || got != want {
		t.Errorf("favourite target %+v, want %+v", got, want)
	}

	m.deps.Favorites = core.NewFavorites([]models.Favorite{{Kind: models.FavoriteCompetition, ID: "esp-la-liga"}})
	if out := m.View(); !strings.Contains(out, "★ La Liga") {
		t.Errorf("a favourite league should be starred in the list:\n%s", out)
	}
}

func TestShiftCSelectsTheCurrentSeasonOfEveryLeague(t *testing.T) {
	m := newCatalog(t)
	press(m, "C")
	if seasons, leagues := m.SelectedCount(); seasons != 2 || leagues != 2 {
		t.Fatalf("selected %d season(s) across %d league(s), want 2 across 2", seasons, leagues)
	}
	press(m, "C")
	if seasons, _ := m.SelectedCount(); seasons != 0 {
		t.Errorf("a second C should deselect them, %d left", seasons)
	}
}

func TestUClearsTheSelection(t *testing.T) {
	m := newCatalog(t)
	press(m, "C")
	press(m, "u")
	if seasons, _ := m.SelectedCount(); seasons != 0 {
		t.Errorf("u should clear the selection, %d left", seasons)
	}
}

func TestSStopsDownloadsOnlyWhileOneIsRunning(t *testing.T) {
	m := newCatalog(t)
	if cmd := press(m, "s"); cmd != nil {
		t.Error("with nothing running, s should do nothing")
	}
	m.Update(core.RefreshQueueMsg{Active: &core.RefreshRequest{CompetitionID: "esp-la-liga"}})
	if _, ok := tuitest.CapturedCmd(press(m, "s")).(core.CancelRefreshMsg); !ok {
		t.Error("s while downloading should ask the app to stop")
	}
}
