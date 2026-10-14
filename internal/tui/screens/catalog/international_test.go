package catalog

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

// intlCatalog lists one club league, a tournament, a qualifier by year,
// and the men's national teams line, with Arcadia's matches stored and
// the 2025 match lists of both competitions fetched.
func intlCatalog(t *testing.T) *Model {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	at := storagetest.Fixed
	for _, c := range []string{"wcq-uefa", "int-friendly"} {
		if err := repos.RecordSyncRun(context.Background(), models.SyncRun{Provider: "espn", CompetitionID: c, Year: 2025,
			StartedAt: at, FinishedAt: at, Status: models.SyncStatusOK}); err != nil {
			t.Fatal(err)
		}
	}
	nations := models.Competition{ID: "national-teams-men", Name: "National teams (men)", Country: "International", Gender: "men", CalendarYear: true}
	deps := core.Deps{
		Ctx: context.Background(), Repos: repos, Theme: styles.Plain(),
		Catalog: []core.CatalogEntry{
			{Competition: laLiga, Seasons: []string{"2026", "2025"}},
			{Competition: storagetest.WorldCup, Seasons: []string{"2026", "2022"}},
			{Competition: storagetest.Qualifying, Seasons: []string{"2026", "2025"}, ByYear: true},
			{Competition: nations, Nations: "men", Seasons: []string{"2026", "2025"},
				Units: map[string][]string{"2026": {"world-cup", "int-friendly"}, "2025": {"wcq-uefa", "int-friendly"}}},
		},
	}
	m := New(deps)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return tuitest.Settle(t, m).(*Model)
}

func TestInternationalEntriesSitUnderTheirOwnHeading(t *testing.T) {
	m := intlCatalog(t)
	out := m.View()
	laliga, heading, wc := strings.Index(out, "La Liga"), strings.Index(out, "International"), strings.Index(out, "FIFA World Cup")
	if laliga < 0 || heading < laliga || wc < heading {
		t.Errorf("want clubs, then the International heading, then tournaments:\n%s", out)
	}
	if !strings.Contains(out, "National teams (men)") {
		t.Errorf("missing the national teams line:\n%s", out)
	}
	// The heading isn't a stop: down from La Liga lands on the World Cup.
	press(m, "down")
	if l, _ := m.currentLeague(); l.Competition.ID != "world-cup" {
		t.Errorf("cursor on %s", l.Competition.ID)
	}
}

// A year of match lists counts as cached once every list it needs was
// fetched — for the national teams line, every competition that year.
func TestYearlyEntriesAreCachedByTheirMatchLists(t *testing.T) {
	m := intlCatalog(t)
	byID := map[string]League{}
	for _, l := range m.leagues {
		byID[l.Competition.ID] = l
	}
	if q := byID["wcq-uefa"]; q.Seasons[1].Cached != true || q.Seasons[0].Cached {
		t.Errorf("qualifying %+v: 2025 fetched, 2026 not", q.Seasons)
	}
	if n := byID["national-teams-men"]; !n.Seasons[1].Cached || n.Seasons[0].Cached || n.Seasons[1].LastRun == nil {
		t.Errorf("national teams %+v: 2025 complete, 2026 not", n.Seasons)
	}
	if wc := byID["world-cup"]; !wc.Seasons[0].Cached {
		t.Errorf("World Cup 2026 has a stored season: %+v", wc.Seasons)
	}
}

func TestTheNationalTeamsLineDownloadsYearsForEveryNation(t *testing.T) {
	m := intlCatalog(t)
	for range 3 {
		press(m, "down")
	}
	if _, ok := m.FavoriteTarget(); ok {
		t.Error("the national teams line isn't something to star")
	}
	if req, ok := m.RefreshRequest(); !ok || req.Nations != "men" {
		t.Errorf("r on the line: %+v", req)
	}
	press(m, "tab")
	press(m, " ")
	press(m, " ")
	reqs := requests(press(m, "enter"))
	if len(reqs) != 1 || reqs[0].Nations != "men" || strings.Join(reqs[0].Seasons, " ") != "2026 2025" {
		t.Errorf("requests %+v", reqs)
	}
}

// C is "this season of every league": the club leagues, not every
// tournament and qualifier too.
func TestSelectCurrentEverywhereLeavesInternationalAlone(t *testing.T) {
	m := intlCatalog(t)
	press(m, "C")
	seasons, leagues := m.SelectedCount()
	if seasons != 1 || leagues != 1 || !m.isSelected("esp-la-liga", "2026") {
		t.Errorf("selected %d seasons in %d leagues", seasons, leagues)
	}
}
