package league

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
		if msg.CompetitionID != m.compID {
			return m, nil
		}
		// Reload from the top: the refresh may have added a season, not
		// just rows to the one already shown. The season on screen is
		// kept rather than snapping back to the newest.
		if season, ok := m.currentSeason(); ok {
			m.fromSeasonID = season.ID
		}
		m.loading = true
		return m, m.loadCompetition()

	case competitionLoadedMsg:
		m.comp = msg.comp
		m.seasons = msg.seasons
		m.seasonIdx = 0
		for i, season := range m.seasons {
			if season.ID == m.fromSeasonID {
				m.seasonIdx = i
				break
			}
		}
		if len(m.seasons) == 0 {
			// No seasons cached at all: nothing left to fetch, so settle
			// into the empty state rather than spinning forever.
			m.loading = false
			m.loaded = true
			return m, nil
		}
		return m, m.loadStandings(m.seasons[m.seasonIdx].ID)

	case standingsLoadedMsg:
		// A stale response for a season the user has already navigated
		// away from must not overwrite the current view.
		if season, ok := m.currentSeason(); !ok || season.ID != msg.seasonID {
			return m, nil
		}
		// A refresh reloads the same season: keep the cursor on the same
		// club, wherever it now sits, rather than throwing the user back
		// to the header.
		selected, hadSelection := m.SelectedRow()
		m.rows = msg.rows
		m.lastSync = msg.lastSync
		m.cursor = headerRow
		for i, r := range m.rows {
			if hadSelection && r.Team.ID == selected.Team.ID {
				m.cursor = i
				break
			}
		}
		m.loading = false
		m.loaded = true
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
	case "home", "g":
		m.cursor = headerRow
	case "end", "G":
		m.cursor = len(m.rows) - 1
	case "pgup":
		m.moveCursor(-10)
	case "pgdown":
		m.moveCursor(10)
	case "left", "h", "shift+tab":
		return m, m.selectSeason(m.seasonIdx + 1) // older
	case "right", "l", "tab":
		return m, m.selectSeason(m.seasonIdx - 1) // newer
	case "enter":
		return m, m.drillDown()
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	// The header sits one step above the first club.
	m.cursor += delta
	if m.cursor < headerRow {
		m.cursor = headerRow
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
}

// selectSeason switches the displayed season. Seasons are ordered most
// recent first, so a lower index is a newer season.
func (m *Model) selectSeason(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.seasons) || idx == m.seasonIdx {
		return nil
	}
	m.seasonIdx = idx
	m.loading = true
	m.rows = nil
	m.lastSync = nil
	m.cursor = headerRow
	return m.loadStandings(m.seasons[idx].ID)
}

// drillDown opens the Team screen for the selected row. League decides what
// drilling into a row means; the root model only moves the stack.
func (m *Model) drillDown() tea.Cmd {
	row, ok := m.SelectedRow()
	if !ok {
		return nil
	}
	season, hasSeason := m.currentSeason()
	if !hasSeason {
		return nil
	}
	if m.deps.Nav.Team == nil {
		return nil
	}
	return core.Push(m.deps.Nav.Team(m.deps, row.Team, m.compID, season.ID))
}
