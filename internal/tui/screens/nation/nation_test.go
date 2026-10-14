package nation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/nation"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var arcadia = models.Team{ID: "nat-arcadia", Name: "Arcadia", Gender: "men", National: true}

func open(t *testing.T) (*nation.Model, storage.Repositories) {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	deps := core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(nil)}
	s := tuitest.Resize(t, tuitest.Settle(t, nation.New(deps, arcadia)), 110, 40)
	return s.(*nation.Model), repos
}

func key(t *testing.T, m *nation.Model, keys ...string) *nation.Model {
	t.Helper()
	var s core.Screen = m
	for _, k := range keys {
		s = tuitest.Key(t, s, k)
	}
	return s.(*nation.Model)
}

func TestOpensOnTheNewestYearWithEveryMatch(t *testing.T) {
	m, _ := open(t)
	out := m.View()
	for _, want := range []string{"Arcadia", "national team · men", "[2026]", "2025", "All years", "[All]", "✓WC", "✓FRI"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q\n%s", want, out)
		}
	}
	if got := len(m.Matches()); got != 5 {
		t.Errorf("%d matches in 2026, want 5 (one still to play)", got)
	}
	if s := m.Summary(); s.Played != 4 || s.Won != 2 || s.Drawn != 1 || s.Lost != 1 {
		t.Errorf("2026 record %+v", s)
	}
	// Newest first; the shoot-out shows both scores, the fixture its
	// kickoff.
	if !strings.Contains(out, "1–1 (3–4 p)") || strings.Index(out, "Borduria") > strings.Index(out, "Quarterfinal") {
		t.Errorf("match list:\n%s", out)
	}
}

func TestYearsIncludeAllYears(t *testing.T) {
	m, _ := open(t)
	m = key(t, m, "left")
	if s := m.Summary(); s.Played != 3 || !strings.Contains(m.View(), "[2025]") {
		t.Errorf("2025: %+v", s)
	}
	m = key(t, m, "right", "right")
	if s := m.Summary(); s.Played != 7 || !strings.Contains(m.View(), "[All years]") {
		t.Errorf("all years: %+v", s)
	}
	if s := m.Summary(); s.Won != 4 || s.Drawn != 2 || s.Lost != 1 || s.Form != "DWWLD" {
		t.Errorf("all years record %+v", s)
	}
}

// Presets and toggles recompute the record from the matches they keep,
// and the choice is saved for the nation.
func TestFilterPresetsAndTogglesRecomputeAndAreSaved(t *testing.T) {
	m, repos := open(t)
	m = key(t, m, "right") // All years
	m = key(t, m, "tab", "right", "space")
	if f := m.Filter(); f.Preset != models.PresetCompetitive {
		t.Fatalf("filter %+v, want Competitive", f)
	}
	if s := m.Summary(); s.Played != 5 || s.Won != 2 || s.Drawn != 2 || s.Lost != 1 {
		t.Errorf("competitive record %+v", s)
	}
	if strings.Contains(m.View(), "✓FRI") || !strings.Contains(m.View(), "☐FRI") {
		t.Errorf("friendlies should be shown unticked:\n%s", m.View())
	}

	// Untick the World Cup: what's left is the qualifiers alone.
	m = key(t, m, "right", "right", "space")
	f := m.Filter()
	if f.Preset != "" || len(f.Competitions) != 1 || f.Competitions[0] != "wcq-uefa" {
		t.Fatalf("filter %+v", f)
	}
	if s := m.Summary(); s.Played != 2 {
		t.Errorf("qualifiers only: %+v", s)
	}
	saved, err := repos.GetTeamMatchFilter(context.Background(), "nat-arcadia")
	if err != nil || len(saved.Competitions) != 1 {
		t.Errorf("saved filter %+v, %v", saved, err)
	}

	// Ticking it again lands back on a preset, shown as such.
	m = key(t, m, "space")
	if f := m.Filter(); f.Preset != models.PresetCompetitive {
		t.Errorf("filter %+v, want it to fold back into Competitive", f)
	}

	// A new screen on the same nation opens with the saved filter.
	deps := core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
	again := tuitest.Settle(t, nation.New(deps, arcadia)).(*nation.Model)
	if again.Filter().Preset != models.PresetCompetitive {
		t.Errorf("reopened with %+v", again.Filter())
	}
}

func TestEnterExpandsTheBoxScore(t *testing.T) {
	m, _ := open(t)
	m = key(t, m, "down", "down") // newest first: the fixture to come, then the quarterfinal
	m = key(t, m, "enter")
	out := m.View()
	if !strings.Contains(out, "Possession") || !strings.Contains(out, "55% – —") {
		t.Errorf("box score should show both sides, the missing side as —:\n%s", out)
	}
	m = key(t, m, "enter")
	if strings.Contains(m.View(), "Possession") {
		t.Error("enter again should close it")
	}
}

func TestFavouriteIsTheNationFromTheHeaderAndTheCompetitionFromARow(t *testing.T) {
	m, _ := open(t)
	target, _ := m.FavoriteTarget()
	if target.Kind != models.FavoriteTeam || target.ID != "nat-arcadia" {
		t.Errorf("header target %+v", target)
	}
	m = key(t, m, "down", "down")
	target, _ = m.FavoriteTarget()
	if target.Kind != models.FavoriteCompetition || target.ID != "world-cup" {
		t.Errorf("row target %+v", target)
	}
	m = key(t, m, "tab")
	if target, _ := m.FavoriteTarget(); target.ID != "nat-arcadia" {
		t.Errorf("filter bar target %+v", target)
	}
	if handled, _ := m.Escape(); !handled {
		t.Error("esc should leave the filter bar first")
	}
}

func TestRefreshAsksForTheNationsMatches(t *testing.T) {
	m, _ := open(t)
	req, ok := m.RefreshRequest()
	if !ok || req.TeamID != "nat-arcadia" || req.Name != "Arcadia" || len(req.Seasons) != 1 || req.Seasons[0] != "2026" {
		t.Errorf("request %+v", req)
	}
	updated, _ := m.Update(core.RefreshQueueMsg{Active: &req})
	if !strings.Contains(updated.View(), "refreshing now") {
		t.Error("the screen should say its matches are being fetched")
	}
}

// Matches arriving for a nation that had none open on the newest year,
// not on "All years" (which was only the empty screen's sole stop).
func TestFirstMatchesOpenOnTheNewestYear(t *testing.T) {
	repos := storagetest.New(t)
	deps := core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
	if err := repos.UpsertTeam(context.Background(), arcadia); err != nil {
		t.Fatal(err)
	}
	s := tuitest.Settle(t, nation.New(deps, arcadia))
	storagetest.SeedInternational(t, repos)
	updated, cmd := s.Update(core.RefreshedMsg{International: true})
	s = tuitest.Drive(t, updated, cmd)
	if !strings.Contains(s.View(), "[2026]") {
		t.Errorf("want the newest year selected:\n%s", s.View())
	}
}

func TestANationWithNoMatchesSaysHowToGetThem(t *testing.T) {
	repos := storagetest.New(t)
	deps := core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
	england := models.Team{ID: "nat-england", Name: "England", Gender: "women", National: true}
	if err := repos.UpsertTeam(context.Background(), england); err != nil {
		t.Fatal(err)
	}
	s := tuitest.Settle(t, nation.New(deps, england))
	out := s.View()
	if !strings.Contains(out, "No matches cached for England") || !strings.Contains(out, `footy refresh --team "england women"`) {
		t.Errorf("empty state:\n%s", out)
	}
}
