package team

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Update implements core.Screen.
func (m *Model) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case core.RefreshedMsg:
		// Reload everything: the refresh re-seeds colors and honours too.
		// The current selection is carried over as the "opened from"
		// hint so the reload lands back on the same tab and row instead
		// of the one the screen was first opened with.
		if comp, ok := m.CurrentCompetition(); ok {
			m.fromCompetitionID = comp.ID
			if rows := m.CurrentRows(); m.seasonIdx >= 0 && m.seasonIdx < len(rows) {
				m.fromSeasonID = rows[m.seasonIdx].Season.ID
			}
		}
		m.loading = true
		return m, m.Init()

	case historyLoadedMsg:
		m.competitions = msg.competitions
		m.history = msg.history
		m.loading = false
		m.loaded = true
		if len(m.competitions) == 0 {
			return m, nil
		}
		m.selectInitial()
		return m, m.refreshRoster()

	case seasonDetailLoadedMsg:
		// Ignore detail that arrived for a selection the user has already
		// moved off.
		if msg.seasonID != m.rosterSeason {
			return m, nil
		}
		m.roster = msg.roster
		m.extras = msg.extras
		m.lastSync = msg.lastSync
		m.cursorSquad = 0
		m.rosterLoading = false
		return m, nil

	case honoursLoadedMsg:
		m.honours = msg.honours
		return m, nil

	case colorsLoadedMsg:
		// A club's brand colors are chosen for shirts, not terminals, so
		// they're passed through styles.TeamTheme rather than used raw.
		// The plain theme is left alone: NO_COLOR means no color, and a
		// club's branding is not an exception to that.
		if !m.theme.NoColor {
			m.colors = &msg.colors
			m.theme = styles.TeamThemeFor(msg.colors.PrimaryHex, msg.colors.SecondaryHex, m.deps.ThemeMode())
			m.branded = true
		}
		return m, nil

	case core.ThemeChangedMsg:
		m.deps.Theme = msg.Theme
		if m.colors != nil {
			m.theme = styles.TeamThemeFor(m.colors.PrimaryHex, m.colors.SecondaryHex, msg.Mode)
		} else {
			// A club with no colors shows in the base theme, which is
			// now the new mode's.
			m.theme = msg.Theme
		}
		return m, nil

	case core.ErrMsg:
		m.err = msg.Err
		m.loading = false
		m.rosterLoading = false
		m.loaded = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.togglePane()
	case "left", "h":
		return m, m.selectCompetition(m.compIdx - 1)
	case "right", "l":
		return m, m.selectCompetition(m.compIdx + 1)
	case "up", "k":
		return m, m.moveCursor(-1)
	case "down", "j":
		return m, m.moveCursor(1)
	case "pgup":
		return m, m.moveCursor(-10)
	case "pgdown":
		return m, m.moveCursor(10)
	case "home", "g":
		return m, m.setCursor(0)
	case "end", "G":
		return m, m.setCursor(m.paneLen() - 1)
	case "enter":
		return m, m.drillDown()
	}
	return m, nil
}

// togglePane moves focus between the seasons table and the squad table.
// Focus only moves to the squad when there is a squad to move to.
func (m *Model) togglePane() {
	if m.focus == paneSeasons && len(m.roster) > 0 {
		m.focus = paneSquad
		return
	}
	m.focus = paneSeasons
}

func (m *Model) paneLen() int {
	if m.focus == paneSquad {
		return len(m.roster)
	}
	return len(m.CurrentRows())
}

func (m *Model) cursor() int {
	if m.focus == paneSquad {
		return m.cursorSquad
	}
	return m.seasonIdx
}

func (m *Model) moveCursor(delta int) tea.Cmd {
	return m.setCursor(m.cursor() + delta)
}

func (m *Model) setCursor(idx int) tea.Cmd {
	n := m.paneLen()
	if n == 0 {
		return nil
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}

	if m.focus == paneSquad {
		m.cursorSquad = idx
		return nil
	}

	if idx == m.seasonIdx {
		return nil
	}
	m.seasonIdx = idx
	// A different season means a different squad.
	return m.refreshRoster()
}

// selectCompetition switches tabs, resetting the season cursor since
// season indices don't correspond across competitions.
func (m *Model) selectCompetition(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.competitions) || idx == m.compIdx {
		return nil
	}
	m.compIdx = idx
	m.seasonIdx = 0
	m.focus = paneSeasons
	return m.refreshRoster()
}

// refreshRoster loads the squad for the currently selected competition and
// season, clearing the previous one so a stale squad is never shown
// attached to the wrong season.
func (m *Model) refreshRoster() tea.Cmd {
	comp, hasComp := m.CurrentCompetition()
	row, hasRow := m.CurrentSeasonRow()
	if !hasComp || !hasRow {
		m.roster = nil
		m.extras = nil
		m.lastSync = nil
		m.rosterSeason = ""
		return nil
	}

	m.roster = nil
	m.extras = nil
	m.lastSync = nil
	m.cursorSquad = 0
	m.rosterSeason = row.Season.ID
	m.rosterLoading = true
	return m.loadSeasonDetail(comp.ID, row.Season.ID)
}

// drillDown opens the Player screen from the squad table, or the League
// table for the selected season from the seasons table.
func (m *Model) drillDown() tea.Cmd {
	if m.focus == paneSquad {
		row, ok := m.SelectedPlayer()
		if !ok {
			return nil
		}
		if m.deps.Nav.Player == nil {
			return core.Status("player view is not available in this build")
		}
		return core.Push(m.deps.Nav.Player(m.deps, row.Player, m.team.ID))
	}

	comp, ok := m.CurrentCompetition()
	if !ok {
		return nil
	}
	if m.deps.Nav.League == nil {
		return core.Status("league view is not available in this build")
	}
	return core.Push(m.deps.Nav.League(m.deps, comp.ID))
}
