// Package player renders one player's career, season by season. Each
// competition is its own tab and totals never sum across competitions.
package player

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// SeasonRow is one season of a player's history.
type SeasonRow struct {
	Stats  models.PlayerSeasonStats
	Team   models.Team
	Season models.Season
	// Detail is nil until fetched, or when there's none.
	Detail    *models.PlayerSeasonDetail
	Standouts []playerstats.Standout
	// XGA is a keeper's estimated xGA from the club's; nil when unknown.
	XGA *float64
}

// Model is the Player screen.
type Model struct {
	deps   core.Deps
	player models.Player

	// fromTeamID only picks the initial competition tab.
	fromTeamID string

	competitions []models.Competition
	history      map[string][]SeasonRow
	// colors is keyed by team ID; the band wears the selected season's.
	colors  map[string]models.TeamColors
	compIdx int
	cursor  int

	width  int
	height int

	group      playerstats.Group
	hasDetail  bool
	refreshing bool

	loading bool
	loaded  bool
	err     error
}

// New constructs the Player screen. teamID may be empty.
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

// Init implements core.Screen. It also queues an automatic detail refresh.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.loadHistory(), core.RequestRefresh(core.RefreshRequest{
		PlayerID: m.player.ID, Name: m.player.Name, Automatic: true,
	}))
}

// FavoriteTarget implements core.Favoritable.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	return core.FavoriteTarget{Kind: models.FavoritePlayer, ID: m.player.ID, Name: m.player.Name}, true
}

// RefreshRequest implements core.RefreshRequester for the whole history.
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

// Career is a player's totals within one competition.
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
	// HasXG, HasShots and HasKeyPasses: any season had data, else show "—".
	HasXG        bool
	HasShots     bool
	HasKeyPasses bool
	// HasMinutes is false if any season played lacks minutes (ESPN lines
	// without detail), since the total would undercount.
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
