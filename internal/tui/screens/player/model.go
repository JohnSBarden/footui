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
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// SeasonRow is one line of a player's history: their stats for a season,
// with the team and season they belong to resolved for display.
type SeasonRow struct {
	Stats  models.PlayerSeasonStats
	Team   models.Team
	Season models.Season
	// Detail is the season's clean sheets, saves, tackles and so on, nil
	// until it has been fetched (or when there's none to fetch).
	Detail *models.PlayerSeasonDetail
	// Standouts are this season's numbers unusual for the position.
	Standouts []playerstats.Standout
	// XGA is a keeper's expected goals against, estimated from the club's;
	// nil when there's no estimate.
	XGA *float64
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

	// group is the player's position, which picks the tiles, chart and
	// columns; hasDetail is whether any season has detail to show them.
	group     playerstats.Group
	hasDetail bool
	// refreshing is set while the player's detail is queued or fetching.
	refreshing bool

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
// Opening a player also asks for their season detail to be brought up to
// date; the refresh is automatic, so a recent copy costs nothing.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadHistory(), core.RequestRefresh(core.RefreshRequest{
		PlayerID: m.player.ID, Name: m.player.Name, Automatic: true,
	}))
}

// FavoriteTarget implements core.Favoritable.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	return core.FavoriteTarget{Kind: models.FavoritePlayer, ID: m.player.ID, Name: m.player.Name}, true
}

// RefreshRequest implements core.RefreshRequester: everything the player
// has stats in, not just the season on screen.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	return core.RefreshRequest{PlayerID: m.player.ID, Name: m.player.Name, History: true}, true
}

type historyLoadedMsg struct {
	competitions []models.Competition
	history      map[string][]SeasonRow
	colors       map[string]models.TeamColors
	group        playerstats.Group
	hasDetail    bool
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
	// HasShots and HasKeyPasses likewise for those counters.
	HasShots     bool
	HasKeyPasses bool
	// HasMinutes reports whether every season played has its minutes. An
	// ESPN line without the player's detail has none, and a total that
	// silently left those seasons out would undercount.
	HasMinutes bool
}

// CareerInCurrentCompetition totals the selected competition's rows.
func (m *Model) CareerInCurrentCompetition() Career {
	c := Career{HasMinutes: true}
	for _, r := range m.CurrentRows() {
		s := r.Stats
		c.Seasons++
		apps, mins := appsOf(r), minutesOf(r)
		c.Appearances += apps
		c.Minutes += mins
		if mins == 0 && apps > 0 {
			c.HasMinutes = false
		}
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
			c.HasKeyPasses = true
		}
	}
	return c
}
