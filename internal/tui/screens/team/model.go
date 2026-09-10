// Package team renders one club's dashboard: its season-by-season record,
// headline stats, and squad.
//
// Everything on this screen is scoped to a single competition at a time,
// selected with the tab bar. That is the screen's central constraint: a
// club's Premier League and Champions League numbers are different numbers
// and must never be summed into one figure. The storage layer already
// keeps rows at (team, competition, season) grain, and this screen groups
// by competition rather than flattening, so the property holds end to end.
package team

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// pane is which of the screen's two tables has the cursor.
type pane int

const (
	paneSeasons pane = iota
	paneSquad
)

// SeasonRow is one row of a team's per-competition history.
type SeasonRow struct {
	Stats  models.TeamSeasonStats
	Season models.Season
}

// RosterRow is one player's line in a squad table.
type RosterRow struct {
	Stats  models.PlayerSeasonStats
	Player models.Player
}

// Model is the Team screen.
type Model struct {
	deps core.Deps
	team models.Team

	// The competition and season the user drilled in from, used to pick
	// the initially selected tab and row so the screen opens showing the
	// same numbers the previous screen was showing.
	fromCompetitionID string
	fromSeasonID      string

	// competitions this team has any cached history in, ordered by tier
	// then name, and the history rows grouped under each.
	competitions []models.Competition
	history      map[string][]SeasonRow
	compIdx      int
	seasonIdx    int

	honours       []models.Trophy
	roster        []RosterRow
	extras        map[string]float64
	rosterSeason  string
	rosterLoading bool
	cursorSquad   int

	focus pane

	width  int
	height int

	// theme is this screen's own theme, which becomes the club's brand
	// theme once its colors load. It is deliberately separate from
	// deps.Theme: deps is what gets handed to screens this one pushes, and
	// a player who turned out for three clubs should not be rendered in
	// one club's colors. Branding stays on the screen it belongs to.
	theme   styles.Theme
	branded bool

	loading bool
	loaded  bool
	err     error
}

// Themed reports whether this screen is rendering in the club's own brand
// colors rather than the neutral theme.
func (m *Model) Themed() bool { return m.branded }

// New constructs the Team screen. competitionID and seasonID say where the
// user came from; either may be empty (e.g. opened from Browse), in which
// case the screen defaults to the most recent season of the team's
// highest-tier competition.
func New(deps core.Deps, team models.Team, competitionID, seasonID string) *Model {
	return &Model{
		deps:              deps,
		theme:             deps.Theme,
		team:              team,
		fromCompetitionID: competitionID,
		fromSeasonID:      seasonID,
		history:           map[string][]SeasonRow{},
		loading:           true,
		width:             100,
		height:            30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string { return m.team.Name }

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadHistory(), m.loadColors(), m.loadHonours())
}

// --- messages ---

type historyLoadedMsg struct {
	competitions []models.Competition
	history      map[string][]SeasonRow
}

// honoursLoadedMsg carries a club's curated honours. An empty list is a
// normal state — the dataset covers only part of every league.
type honoursLoadedMsg struct {
	honours []models.Trophy
}

// colorsLoadedMsg carries a club's brand colors. Absent colors are a
// normal state, not an error, so the message simply isn't sent.
type colorsLoadedMsg struct {
	colors models.TeamColors
}

// seasonDetailLoadedMsg carries everything scoped to the selected
// (competition, season): the squad and the long-tail team stats. They load
// together because the same selection change invalidates both.
type seasonDetailLoadedMsg struct {
	seasonID      string
	competitionID string
	honours       []models.Trophy
	roster        []RosterRow
	extras        map[string]float64
}

// --- accessors used by the view and by tests ---

// CurrentCompetition returns the competition whose tab is selected.
func (m *Model) CurrentCompetition() (models.Competition, bool) {
	if m.compIdx < 0 || m.compIdx >= len(m.competitions) {
		return models.Competition{}, false
	}
	return m.competitions[m.compIdx], true
}

// CurrentRows returns the season rows for the selected competition.
func (m *Model) CurrentRows() []SeasonRow {
	comp, ok := m.CurrentCompetition()
	if !ok {
		return nil
	}
	return m.history[comp.ID]
}

// CurrentSeasonRow returns the season row under the cursor.
func (m *Model) CurrentSeasonRow() (SeasonRow, bool) {
	rows := m.CurrentRows()
	if m.seasonIdx < 0 || m.seasonIdx >= len(rows) {
		return SeasonRow{}, false
	}
	return rows[m.seasonIdx], true
}

// SelectedPlayer returns the squad row under the cursor.
func (m *Model) SelectedPlayer() (RosterRow, bool) {
	if m.cursorSquad < 0 || m.cursorSquad >= len(m.roster) {
		return RosterRow{}, false
	}
	return m.roster[m.cursorSquad], true
}
