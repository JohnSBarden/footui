// Package catalog is the league catalog: every competition footy can
// fetch, which of its seasons are already cached and how fresh they are,
// and a way to pick seasons across leagues and download them in one go.
//
// It is the in-app counterpart to `footy refresh --competition X --season
// Y`. The screen never fetches anything itself — it hands the root app a
// list of core.RefreshRequests, and the app's refresh queue runs them and
// reports progress on the status line, so the user can leave this screen
// and keep browsing while a download runs.
package catalog

import (
	"slices"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// Title is the catalog's breadcrumb label. The root app compares against
// it so the catalog key doesn't push a second catalog on top of the first.
const Title = "Download leagues"

// pane is which list has keyboard focus.
type pane int

const (
	paneLeagues pane = iota
	paneSeasons
)

// SeasonState is what's known locally about one available season.
type SeasonState struct {
	// Year is the bare start year ("2025").
	Year   string
	Cached bool
	// LastRun is the most recent refresh attempt, nil if never tried.
	LastRun *models.SyncRun
}

// League is one catalog entry with its local state.
type League struct {
	Competition models.Competition
	Seasons     []SeasonState
	// Nations is set on a national teams line: its seasons are calendar
	// years of every international competition of that gender.
	Nations string
}

// international reports whether the entry belongs under the catalog's
// International heading.
func (l League) international() bool { return l.Nations != "" || l.Competition.International() }

// cachedCount is how many of the league's seasons are in the cache.
func (l League) cachedCount() int {
	n := 0
	for _, s := range l.Seasons {
		if s.Cached {
			n++
		}
	}
	return n
}

// Model is the catalog screen.
type Model struct {
	deps core.Deps

	leagues []League
	league  int // cursor in the league list
	season  int // cursor in the highlighted league's season list
	focus   pane

	// selected holds the chosen seasons, keyed by competition ID then
	// year. Selections survive moving between leagues, so one download
	// can span several competitions.
	selected map[string]map[string]bool

	// active and pending mirror the root app's refresh queue, so seasons
	// already on their way are marked rather than offered again.
	active  *core.RefreshRequest
	pending []core.RefreshRequest

	loading bool
	err     error

	width  int
	height int
}

// New constructs the catalog over deps.Catalog.
func New(deps core.Deps) *Model {
	return &Model{
		deps:     deps,
		selected: map[string]map[string]bool{},
		loading:  true,
		width:    100,
		height:   30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string { return Title }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	km := core.KeyMap{
		Short: []key.Binding{keys.Move, keys.CatalogPane, keys.Toggle, keys.Download},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.CatalogPane},
			{keys.Toggle, keys.SelectMissing, keys.SelectCurrent, keys.SelectCurrentEverywhere},
			{keys.ClearSelection, keys.Download},
		},
	}
	if m.active != nil {
		km.Short = append(km.Short, keys.StopDownloads)
		km.Full[2] = append(km.Full[2], keys.StopDownloads)
	}
	return km
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd { return m.load() }

// RefreshTarget implements core.Refreshable: r refreshes the current
// season of the highlighted league, as it does on the League screen.
func (m *Model) RefreshTarget() (string, bool) {
	l, ok := m.currentLeague()
	if !ok || l.Nations != "" {
		return "", false
	}
	return l.Competition.ID, true
}

// RefreshRequest implements core.RefreshRequester: on the national teams
// line, r fetches this year's and last year's match lists.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	l, ok := m.currentLeague()
	if !ok || l.Nations == "" {
		return core.RefreshRequest{}, false
	}
	return core.RefreshRequest{Nations: l.Nations, Name: l.Competition.Name}, true
}

// FavoriteTarget implements core.Favoritable: the highlighted league,
// whichever pane has focus — the seasons pane belongs to it too.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	l, ok := m.currentLeague()
	if !ok || l.Nations != "" {
		return core.FavoriteTarget{}, false
	}
	c := l.Competition
	return core.FavoriteTarget{Kind: models.FavoriteCompetition, ID: c.ID, Name: c.Name}, true
}

func (m *Model) currentLeague() (League, bool) {
	if m.league < 0 || m.league >= len(m.leagues) {
		return League{}, false
	}
	return m.leagues[m.league], true
}

// SelectedCount reports how many seasons are selected, across how many
// leagues.
func (m *Model) SelectedCount() (seasons, leagues int) {
	for _, years := range m.selected {
		if len(years) > 0 {
			leagues++
			seasons += len(years)
		}
	}
	return seasons, leagues
}

type loadedMsg struct{ leagues []League }

// load reads what's cached for every catalog entry. It is a few dozen
// indexed lookups against a local database, so it runs in one command.
func (m *Model) load() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		// International match lists are fetched by the year, shared by
		// every competition-season and nation; their runs are read once.
		yearRuns := map[string]models.SyncRun{}
		if runs, err := deps.Repos.LatestYearRuns(deps.Ctx); err == nil {
			for _, r := range runs {
				yearRuns[r.CompetitionID+"/"+strconv.Itoa(r.Year)] = r
			}
		}
		leagues := make([]League, 0, len(deps.Catalog))
		for _, entry := range deps.Catalog {
			if entry.Nations != "" || entry.ByYear {
				leagues = append(leagues, byYear(entry, yearRuns))
				continue
			}
			cached, err := deps.Repos.ListSeasons(deps.Ctx, entry.Competition.ID)
			if err != nil {
				return core.ErrMsg{Err: err}
			}
			have := make(map[string]bool, len(cached))
			for _, s := range cached {
				have[s.Label] = true
			}

			l := League{Competition: entry.Competition}
			for _, year := range entry.Seasons {
				st := SeasonState{Year: year, Cached: have[year]}
				run, err := deps.Repos.LatestSyncRun(deps.Ctx, entry.Competition.ID, entry.Competition.ID+":"+year)
				if err == nil {
					st.LastRun = &run
				}
				l.Seasons = append(l.Seasons, st)
			}
			leagues = append(leagues, l)
		}
		return loadedMsg{leagues: leagues}
	}
}

// byYear is the local state of an entry whose seasons are calendar years
// of match lists: a year is cached once every list it needs was fetched,
// and its last run is the newest of theirs — or a failure, if one failed.
func byYear(entry core.CatalogEntry, runs map[string]models.SyncRun) League {
	l := League{Competition: entry.Competition, Nations: entry.Nations}
	for _, year := range entry.Seasons {
		comps := entry.Units[year]
		if entry.Nations == "" {
			comps = []string{entry.Competition.ID}
		}
		st := SeasonState{Year: year, Cached: len(comps) > 0}
		for _, c := range comps {
			run, ok := runs[c+"/"+year]
			if !ok || !run.Succeeded() {
				st.Cached = false
			}
			if !ok {
				continue
			}
			switch {
			case st.LastRun == nil, !run.Succeeded() && st.LastRun.Succeeded(),
				run.Succeeded() == st.LastRun.Succeeded() && run.FinishedAt.After(st.LastRun.FinishedAt):
				r := run
				st.LastRun = &r
			}
		}
		l.Seasons = append(l.Seasons, st)
	}
	return l
}

// queued reports whether a season is waiting in, or running on, the
// app's refresh queue. An empty season list in a request is the current
// season, which is the first one the catalog lists.
func (m *Model) queued(l League, year string, current string) (running, waiting bool) {
	compID := l.Competition.ID
	matches := func(r core.RefreshRequest) bool {
		if l.Nations != "" {
			return r.Nations == l.Nations && slices.Contains(r.Seasons, year)
		}
		if r.CompetitionID != compID {
			return false
		}
		if len(r.Seasons) == 0 {
			return year == current
		}
		for _, s := range r.Seasons {
			if s == year {
				return true
			}
		}
		return false
	}
	if m.active != nil && matches(*m.active) {
		return true, false
	}
	for _, r := range m.pending {
		if matches(r) {
			return false, true
		}
	}
	return false, false
}
