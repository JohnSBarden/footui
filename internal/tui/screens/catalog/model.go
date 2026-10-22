// Package catalog lists every fetchable competition and its cached seasons.
// It never fetches itself: it hands core.RefreshRequests to the root app's
// queue, so downloads continue after leaving the screen.
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

// Title is the breadcrumb label; the root app compares against it to avoid
// pushing a second catalog.
const Title = "Download leagues"

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
	// Nations is set on a national teams line, whose seasons are calendar
	// years.
	Nations string
	// Base marks the starter set, which C selects.
	Base bool
}

func (l League) international() bool { return l.Nations != "" || l.Competition.International() }

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

	// selected is keyed by competition ID then year; it spans leagues.
	selected map[string]map[string]bool

	// active and pending mirror the root app's refresh queue.
	active  *core.RefreshRequest
	pending []core.RefreshRequest

	loading bool
	// held are keys pressed while loading, replayed once loaded.
	held []tea.KeyMsg
	err  error

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

// RefreshTarget implements core.Refreshable for the highlighted league.
func (m *Model) RefreshTarget() (string, bool) {
	l, ok := m.currentLeague()
	if !ok || l.Nations != "" {
		return "", false
	}
	return l.Competition.ID, true
}

// RefreshRequest implements core.RefreshRequester for a national teams line.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	l, ok := m.currentLeague()
	if !ok || l.Nations == "" {
		return core.RefreshRequest{}, false
	}
	return core.RefreshRequest{Nations: l.Nations, Name: l.Competition.Name}, true
}

// FavoriteTarget implements core.Favoritable: the highlighted league, from
// either pane.
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

// SelectedCount reports selected seasons and the leagues they span.
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

// load is a few dozen indexed lookups, so it runs in one command.
func (m *Model) load() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		// Year runs are shared by every by-year entry; read them once.
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

			l := League{Competition: entry.Competition, Base: entry.Base}
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

// byYear: a year is cached once every list it needs was fetched; its last
// run is the newest, but a failure wins.
func byYear(entry core.CatalogEntry, runs map[string]models.SyncRun) League {
	l := League{Competition: entry.Competition, Nations: entry.Nations, Base: entry.Base}
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

// queued checks the refresh queue. A request with no seasons means
// current.
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
