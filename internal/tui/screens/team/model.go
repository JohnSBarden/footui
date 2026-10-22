// Package team renders one club's season record, stats and squad. Numbers
// are scoped to one competition tab at a time and must never be summed
// across competitions.
package team

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/scorestrip"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// now is the clock the scores along the top are measured against; tests
// pin it.
var now = time.Now

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

	// Where the user drilled in from; picks the initial tab and row.
	fromCompetitionID string
	fromSeasonID      string

	// competitions this team has any cached history in, ordered by tier
	// then name, and the history rows grouped under each.
	competitions []models.Competition
	history      map[string][]SeasonRow
	compIdx      int
	seasonIdx    int

	honours []models.Trophy

	// recent feeds the form line: last finished games, all competitions.
	recent       []models.Match
	recentLoaded bool
	strip        scorestrip.Strip
	// fetching and queued: recent games are being fetched / waiting to be.
	fetching bool
	queued   bool

	// lastSync is nil when the selection has never been refreshed.
	lastSync      *models.SyncRun
	roster        []RosterRow
	extras        map[string]float64
	rosterSeason  string
	rosterLoading bool
	cursorSquad   int

	focus pane

	width  int
	height int

	// theme is kept separate from deps.Theme so club branding doesn't leak
	// into screens pushed from here (e.g. a player who played for others).
	theme   styles.Theme
	branded bool
	// colors are kept to rebuild the theme on a light/dark mode switch.
	colors *models.TeamColors

	loading bool
	loaded  bool
	err     error
}

// Themed reports whether the screen is in the club's brand colours.
func (m *Model) Themed() bool { return m.branded }

// ScreenTheme implements core.ThemedScreen.
func (m *Model) ScreenTheme() styles.Theme { return m.theme }

// New constructs the Team screen. Empty competitionID/seasonID default to
// the latest season of the team's highest-tier competition.
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

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	km := core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Competition, keys.Pane, keys.Open, keys.Matches},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End, keys.Competition},
			{keys.Pane, keys.Open, keys.Matches},
		},
	}
	if !m.strip.Empty() {
		km.Short = append(km.Short, keys.Scores)
		km.Full = append(km.Full, []key.Binding{keys.Scores})
	}
	return km
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.checkRecent())
}

func (m *Model) load() tea.Cmd {
	return tea.Batch(m.loadHistory(), m.loadColors(), m.loadHonours(), m.loadRecent())
}

// RefreshRequest implements core.RefreshRequester. The origin competition
// is included so a club with nothing cached still gets that league's table.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	return core.RefreshRequest{TeamID: m.team.ID, Name: m.team.Name, CompetitionID: m.fromCompetitionID, History: true}, true
}

// RecentGames returns the strip's games.
func (m *Model) RecentGames() []models.Match { return m.recent }

// FavoriteTarget implements core.Favoritable: always the club, never the
// player under the cursor.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	return core.FavoriteTarget{Kind: models.FavoriteTeam, ID: m.team.ID, Name: m.team.Name}, true
}

// RefreshTarget implements core.Refreshable: the selected tab, else the
// competition the team was opened from.
func (m *Model) RefreshTarget() (string, bool) {
	if comp, ok := m.CurrentCompetition(); ok {
		return comp.ID, true
	}
	return m.fromCompetitionID, m.fromCompetitionID != ""
}

// --- messages ---

type historyLoadedMsg struct {
	competitions []models.Competition
	history      map[string][]SeasonRow
}

// honoursLoadedMsg: an empty list is normal; the dataset is partial.
type honoursLoadedMsg struct {
	honours []models.Trophy
}

// colorsLoadedMsg isn't sent when a club has no colours.
type colorsLoadedMsg struct {
	colors models.TeamColors
}

// seasonDetailLoadedMsg loads squad and extras together because the same
// selection change invalidates both.
type seasonDetailLoadedMsg struct {
	seasonID      string
	competitionID string
	lastSync      *models.SyncRun
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
