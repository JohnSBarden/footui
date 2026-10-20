package league

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
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
		// A nation's refresh brings in the yearly match lists every
		// international competition's seasons are made of.
		if msg.CompetitionID != m.compID && !(msg.International && m.comp.International()) {
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
		group := m.Group()
		m.rows = msg.rows
		m.lastSync = msg.lastSync
		m.colors, m.xpts = msg.colors, msg.xpts
		m.strip = msg.strip
		m.matches, m.matchTeams = displayOrder(msg.matches), msg.matchTeams
		m.setGroups(group)
		m.cursor = headerRow
		for i, r := range m.visibleRows() {
			if hadSelection && r.Team.ID == selected.Team.ID {
				m.cursor = i
				break
			}
		}
		// A season with fixtures but no table (friendlies, a knockout-only
		// finals) opens on its fixtures.
		if len(m.rows) == 0 && len(m.matches) > 0 {
			m.matchesView = true
		}
		if !m.matchesShown {
			m.matchCursor = defaultMatch(m.matches)
		}
		m.matchCursor = min(m.matchCursor, max(len(m.matches)-1, 0))
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
	if m.strip.HandleKey(msg.String(), m.width) {
		return m, nil
	}
	if m.comp.International() {
		switch msg.String() {
		case "m":
			m.matchesView = !m.matchesView
			m.matchesShown = true
			return m, nil
		case "[", "]":
			if len(m.groups) > 1 && !m.matchesView {
				step := 1
				if msg.String() == "[" {
					step = -1
				}
				m.groupIdx = (m.groupIdx + step + len(m.groups)) % len(m.groups)
				m.cursor = headerRow
			}
			return m, nil
		}
		if m.matchesView {
			switch msg.String() {
			case "up", "k":
				m.matchCursor = max(m.matchCursor-1, 0)
				return m, nil
			case "down", "j":
				m.matchCursor = min(m.matchCursor+1, max(len(m.matches)-1, 0))
				return m, nil
			case "pgup":
				m.matchCursor = max(m.matchCursor-10, 0)
				return m, nil
			case "pgdown":
				m.matchCursor = min(m.matchCursor+10, max(len(m.matches)-1, 0))
				return m, nil
			case "home", "g":
				m.matchCursor = 0
				return m, nil
			case "end", "G":
				m.matchCursor = max(len(m.matches)-1, 0)
				return m, nil
			case "enter":
				return m, nil
			}
		}
	}
	switch msg.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "home", "g":
		m.cursor = headerRow
	case "end", "G":
		m.cursor = len(m.visibleRows()) - 1
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
	rows := m.visibleRows()
	if len(rows) == 0 {
		return
	}
	// The header sits one step above the first club.
	m.cursor = min(max(m.cursor+delta, headerRow), len(rows)-1)
}

// setGroups reads a tournament table's groups from its rows, keeping the
// group that was on screen, or else opening on the group of a starred
// nation — the one the user follows the tournament for.
func (m *Model) setGroups(keep string) {
	m.groups = nil
	seen := map[string]bool{}
	starred := ""
	for _, r := range m.rows {
		g := r.Stats.GroupLabel
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		m.groups = append(m.groups, g)
	}
	for _, r := range m.rows {
		if r.Stats.GroupLabel != "" && m.deps.Favorites.Has(models.FavoriteTeam, r.Team.ID) {
			starred = r.Stats.GroupLabel
			break
		}
	}
	m.groupIdx = 0
	for i, g := range m.groups {
		if g == keep || (keep == "" && g == starred) {
			m.groupIdx = i
		}
	}
}

// defaultMatch is where the fixtures view opens: the next match to be
// played, or the last one when it's all over.
func defaultMatch(matches []models.Match) int {
	for i, mt := range matches {
		if !mt.Finished() && mt.Status != models.MatchCancelled {
			return i
		}
	}
	return max(len(matches)-1, 0)
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
	m.matches, m.matchesShown = nil, false
	m.groups = nil
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
