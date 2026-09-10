// Package league renders a competition's standings table for its two most
// recent cached seasons.
//
// The two-season cap is a product requirement, not a performance
// workaround: a league view is for "how does this season look", and deeper
// history belongs on the Team screen, which slices by competition.
package league

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// maxSeasons is how many recent seasons the League screen exposes.
const maxSeasons = 2

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

	comp      models.Competition
	seasons   []models.Season
	seasonIdx int

	rows   []Row
	cursor int

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
	return &Model{
		deps:    deps,
		compID:  competitionID,
		loading: true,
		width:   100,
		height:  30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string {
	if m.comp.Name != "" {
		return m.comp.Name
	}
	return m.compID
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return m.loadCompetition()
}

// --- messages ---

type competitionLoadedMsg struct {
	comp    models.Competition
	seasons []models.Season
}

type standingsLoadedMsg struct {
	seasonID string
	rows     []Row
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
		if len(seasons) > maxSeasons {
			seasons = seasons[:maxSeasons]
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
		return standingsLoadedMsg{seasonID: seasonID, rows: rows}
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
