// Package league renders a competition's standings table, one cached
// season at a time, with every cached season selectable from a tab bar.
package league

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// Row pairs a standings stat line with the team it belongs to. Stats rows
// carry only team IDs, so the screen resolves names once at load time
// rather than per render.
type Row struct {
	Stats models.TeamSeasonStats
	Team  models.Team
}

// Model is the League screen.
type Model struct {
	deps   core.Deps
	compID string
	// fromSeasonID is the season to open on, when the user drilled in
	// from a particular one; empty opens the most recent.
	fromSeasonID string

	comp      models.Competition
	seasons   []models.Season
	seasonIdx int

	rows   []Row
	cursor int

	// lastSync is the most recent refresh attempt for the displayed
	// season, or nil when it has never been attempted.
	lastSync *models.SyncRun

	width  int
	height int

	loading bool
	// loaded distinguishes "still fetching" from "fetched, and there
	// genuinely is nothing cached" — the two need different empty states.
	loaded bool
	err    error
}

// New constructs the League screen for a competition ID. Data loading
// happens in Init, so constructing a screen is cheap and never blocks the
// event loop.
func New(deps core.Deps, competitionID string) *Model {
	return NewAt(deps, competitionID, "")
}

// NewAt constructs the League screen opened on seasonID, falling back to
// the most recent cached season when that one isn't cached.
func NewAt(deps core.Deps, competitionID, seasonID string) *Model {
	return &Model{
		deps:         deps,
		compID:       competitionID,
		fromSeasonID: seasonID,
		loading:      true,
		width:        100,
		height:       30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string {
	if m.comp.Name != "" {
		return m.comp.Name
	}
	return m.compID
}

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	return core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Season, keys.Open},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End, keys.Season, keys.Open},
		},
	}
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return m.loadCompetition()
}

// RefreshTarget implements core.Refreshable: a refresh from the League
// screen fetches the competition it is showing.
func (m *Model) RefreshTarget() (string, bool) {
	return m.compID, true
}

// --- messages ---

type competitionLoadedMsg struct {
	comp    models.Competition
	seasons []models.Season
}

type standingsLoadedMsg struct {
	seasonID string
	rows     []Row
	// lastSync is nil when this competition-season has never been
	// refreshed — a normal state for data loaded before sync logging
	// existed, or seeded by hand.
	lastSync *models.SyncRun
}

// --- commands ---

// loadCompetition fetches the competition row and its cached seasons.
func (m *Model) loadCompetition() tea.Cmd {
	deps, compID := m.deps, m.compID
	return func() tea.Msg {
		comp, err := deps.Repos.GetCompetition(deps.Ctx, compID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		seasons, err := deps.Repos.ListSeasons(deps.Ctx, compID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return competitionLoadedMsg{comp: comp, seasons: seasons}
	}
}

// loadStandings fetches one season's standings and resolves every team name
// in a single batch query.
func (m *Model) loadStandings(seasonID string) tea.Cmd {
	deps, compID := m.deps, m.compID
	return func() tea.Msg {
		stats, err := deps.Repos.Standings(deps.Ctx, compID, seasonID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		ids := make([]string, 0, len(stats))
		for _, s := range stats {
			ids = append(ids, s.TeamID)
		}
		teams, err := deps.Repos.GetTeams(deps.Ctx, ids)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		rows := make([]Row, 0, len(stats))
		for _, s := range stats {
			team, ok := teams[s.TeamID]
			if !ok {
				// Render the ID rather than dropping the row: a missing
				// team row is a data defect worth seeing, not worth
				// hiding a league position over.
				team = models.Team{ID: s.TeamID, Name: s.TeamID}
			}
			rows = append(rows, Row{Stats: s, Team: team})
		}

		// A missing sync log entry is expected, not an error, so it never
		// fails the load.
		var lastSync *models.SyncRun
		if run, err := deps.Repos.LatestSyncRun(deps.Ctx, compID, seasonID); err == nil {
			lastSync = &run
		}

		return standingsLoadedMsg{seasonID: seasonID, rows: rows, lastSync: lastSync}
	}
}

// currentSeason returns the season currently being displayed, if any.
func (m *Model) currentSeason() (models.Season, bool) {
	if m.seasonIdx < 0 || m.seasonIdx >= len(m.seasons) {
		return models.Season{}, false
	}
	return m.seasons[m.seasonIdx], true
}

// SelectedRow returns the row under the cursor, for drill-down.
func (m *Model) SelectedRow() (Row, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return Row{}, false
	}
	return m.rows[m.cursor], true
}
