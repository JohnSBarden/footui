package player

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// Update implements core.Screen.
func (m *Model) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case core.ThemeChangedMsg:
		// This screen draws its own content, so it adopts the new theme
		// rather than leaving text styled for the old background.
		m.deps.Theme = msg.Theme
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case core.RefreshedMsg:
		if _, ok := m.history[msg.CompetitionID]; !ok {
			return m, nil
		}
		m.loading = true
		return m, m.loadHistory()

	case historyLoadedMsg:
		reload := m.loaded
		prevComp, _ := m.CurrentCompetition()
		prevCursor := m.cursor
		m.competitions = msg.competitions
		m.history = msg.history
		m.loading = false
		m.loaded = true
		if len(m.competitions) == 0 {
			return m, nil
		}
		m.selectInitial()
		if reload {
			// After a refresh, stay on the tab and row the user was on.
			for i, c := range m.competitions {
				if c.ID == prevComp.ID {
					m.compIdx = i
					m.cursor = min(prevCursor, len(m.CurrentRows())-1)
				}
			}
		}
		return m, nil

	case core.ErrMsg:
		m.err = msg.Err
		m.loading = false
		m.loaded = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-10)
	case "pgdown":
		m.moveCursor(10)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.CurrentRows()) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "left", "h", "shift+tab":
		m.selectCompetition(m.compIdx - 1)
	case "right", "l", "tab":
		m.selectCompetition(m.compIdx + 1)
	case "enter":
		return m, m.openTeam()
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	rows := m.CurrentRows()
	if len(rows) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
}

// selectCompetition switches tabs, resetting the cursor since season
// indices don't correspond across competitions.
func (m *Model) selectCompetition(idx int) {
	if idx < 0 || idx >= len(m.competitions) || idx == m.compIdx {
		return
	}
	m.compIdx = idx
	m.cursor = 0
}

// openTeam drills into the club the selected season was played for — the
// natural next step from a player's line, and the way back into a squad
// the user may not have come from.
func (m *Model) openTeam() tea.Cmd {
	row, ok := m.SelectedRow()
	if !ok {
		return nil
	}
	if m.deps.Nav.Team == nil {
		return nil
	}
	return core.Push(m.deps.Nav.Team(m.deps, row.Team, row.Stats.CompetitionID, row.Season.ID))
}
