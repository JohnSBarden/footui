package dashboard

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// Update implements core.Screen.
func (m *Model) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case core.ThemeChangedMsg:
		m.deps.Theme = msg.Theme
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case core.RefreshedMsg, core.FavoritesChangedMsg:
		return m, m.load()

	case core.FrameMsg:
		m.frame++
		return m, nil

	case core.RefreshQueueMsg:
		m.active, m.pending = map[string]bool{}, map[string]bool{}
		if msg.Active != nil {
			m.active[msg.Active.Key()] = true
		}
		for _, r := range msg.Pending {
			m.pending[r.Key()] = true
		}
		return m, nil

	case cardsLoadedMsg:
		m.cards = msg.cards
		m.strip = msg.strip
		m.loading = false
		m.err = nil
		if m.cursor >= len(m.cards) {
			m.cursor = len(m.cards) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, nil

	case core.ErrMsg:
		m.err = msg.Err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case core.LogPanelMsg:
		m.logOpen = msg.Open
		return m, nil

	case core.ClickMsg:
		// Clicking the selected card opens it.
		i, ok := msg.ClickedIndex("card")
		if !ok || i >= len(m.cards) {
			return m, nil
		}
		if i == m.cursor {
			return m, m.open()
		}
		m.cursor, m.frame = i, 0
		return m, nil
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	if m.strip.HandleKey(msg.String(), m.width) {
		return m, nil
	}
	// A newly selected card's glint starts from its top-left corner.
	before := m.cursor
	defer func() {
		if m.cursor != before {
			m.frame = 0
		}
	}()
	switch msg.String() {
	case "left", "h":
		m.move(0, -1)
	case "right", "l":
		m.move(0, 1)
	case "up", "k":
		m.move(-1, 0)
	case "down", "j":
		m.move(1, 0)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		if len(m.cards) > 0 {
			m.cursor = len(m.cards) - 1
		}
	case "enter":
		return m, m.open()
	}
	return m, nil
}

// open drills into the same screen Browse would, at the card's season.
func (m *Model) open() tea.Cmd {
	c, ok := m.Selected()
	if !ok {
		return nil
	}
	if c.Missing {
		return core.Status(c.ID + " isn't cached — press d to download its league, or f to remove it")
	}
	nav := m.deps.Nav
	switch c.Kind {
	case models.FavoriteCompetition:
		if nav.League == nil {
			return nil
		}
		return core.Push(nav.League(m.deps, c.ID, c.Season.ID))
	case models.FavoriteTeam:
		if nav.Team == nil {
			return nil
		}
		return core.Push(nav.Team(m.deps, c.Team, c.Competition.ID, c.Season.ID))
	default:
		if nav.Player == nil {
			return nil
		}
		return core.Push(nav.Player(m.deps, c.Player, c.Team.ID))
	}
}
