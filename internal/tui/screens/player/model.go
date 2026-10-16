// Package player renders one player's career: a season-by-season stat
// table sliced by competition, with headline numbers for the selected
// season.
//
// Like the Team screen, competitions are tabs rather than merged rows —
// a striker's league and cup returns are different numbers and summing
// them would misrepresent both. Career totals are shown per competition
// for the same reason.
package player

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// SeasonRow is one line of a player's history: their stats for a season,
// with the team and season they belong to resolved for display.
type SeasonRow struct {
	Stats  models.PlayerSeasonStats
	Team   models.Team
	Season models.Season
}

// Model is the Player screen.
type Model struct {
	deps   core.Deps
	player models.Player

	// fromTeamID is the club the user drilled in from, used only to pick
	// the initially selected competition tab.
	fromTeamID string

	competitions []models.Competition
	history      map[string][]SeasonRow
	// colors are the brand colours of every club the player has played
	// for, keyed by team ID; the header band wears the selected season's.
	colors  map[string]models.TeamColors
	compIdx int
	cursor  int

	width  int
	height int

	loading bool
	loaded  bool
	err     error
}

// New constructs the Player screen. teamID may be empty (opened from
// Browse), in which case the first competition tab is selected.
func New(deps core.Deps, p models.Player, teamID string) *Model {
	return &Model{
		deps:       deps,
		player:     p,
		fromTeamID: teamID,
		history:    map[string][]SeasonRow{},
		loading:    true,
		width:      100,
		height:     30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string { return m.player.Name }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	openClub := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open club"))
	return core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Competition, openClub},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End, keys.Competition, openClub},
		},
	}
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd { return m.loadHistory() }

// FavoriteTarget implements core.Favoritable.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	return core.FavoriteTarget{Kind: models.FavoritePlayer, ID: m.player.ID, Name: m.player.Name}, true
}

// RefreshTarget implements core.Refreshable: the competition whose tab is
// selected.
func (m *Model) RefreshTarget() (string, bool) {
	comp, ok := m.CurrentCompetition()
	return comp.ID, ok
}

type historyLoadedMsg struct {
	competitions []models.Competition
	history      map[string][]SeasonRow
	colors       map[string]models.TeamColors
}

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

// SelectedRow returns the season row under the cursor.
func (m *Model) SelectedRow() (SeasonRow, bool) {
	rows := m.CurrentRows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return SeasonRow{}, false
	}
	return rows[m.cursor], true
}

// Career is a player's totals within one competition. Totals are only ever
// computed within a single competition — see the package comment.
type Career struct {
	Seasons     int
	Appearances int
	Minutes     int
	Goals       int
	Assists     int
	Shots       int
	KeyPasses   int
	XG          float64
	XA          float64
	// HasXG reports whether any season contributed expected-goals data, so
	// a career with none shows "—" rather than a misleading 0.0.
	HasXG bool
	// HasShots likewise for the shot-based counters.
	HasShots bool
}

// CareerInCurrentCompetition totals the selected competition's rows.
func (m *Model) CareerInCurrentCompetition() Career {
	var c Career
	for _, r := range m.CurrentRows() {
		s := r.Stats
		c.Seasons++
		c.Appearances += s.Appearances
		c.Minutes += s.Minutes
		c.Goals += s.Goals
		c.Assists += s.Assists
		if s.XG != nil {
			c.XG += *s.XG
			c.HasXG = true
		}
		if s.XA != nil {
			c.XA += *s.XA
			c.HasXG = true
		}
		if s.Shots != nil {
			c.Shots += *s.Shots
			c.HasShots = true
		}
		if s.KeyPasses != nil {
			c.KeyPasses += *s.KeyPasses
			c.HasShots = true
		}
	}
	return c
}
