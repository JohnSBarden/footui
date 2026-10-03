package browse

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
		// Whatever the query, the index it searches just changed.
		m.loading = true
		return m, m.search(m.query)

	case resultsMsg:
		// Drop results for a query the user has already typed past.
		if msg.query != m.query {
			return m, nil
		}
		m.results = msg.results
		m.cursor = 0
		m.loading = false
		return m, nil

	case core.ErrMsg:
		m.err = msg.Err
		m.loading = false
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	if !m.input.Focused() {
		return m.handleListKey(msg)
	}
	switch msg.String() {
	case "tab":
		if len(m.results) > 0 {
			m.input.Blur()
		}
		return m, nil
	case "up", "ctrl+p":
		m.moveCursor(-1)
		return m, nil
	case "down", "ctrl+n":
		m.moveCursor(1)
		return m, nil
	case "pgup":
		m.moveCursor(-10)
		return m, nil
	case "pgdown":
		m.moveCursor(10)
		return m, nil
	case "enter":
		return m, m.open()
	}

	// Everything else is text entry. The query is re-run only when the
	// text actually changed, so arrow keys inside the input don't trigger
	// redundant queries.
	before := m.input.Value()
	updated, inputCmd := m.input.Update(msg)
	m.input = updated

	if after := m.input.Value(); after != before {
		m.query = after
		m.loading = true
		return m, tea.Batch(inputCmd, m.search(after))
	}
	return m, inputCmd
}

// handleListKey handles keys while the results list has focus. The search
// box isn't listening, so vim-style movement is safe here.
func (m *Model) handleListKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "ctrl+p":
		m.moveCursor(-1)
	case "down", "j", "ctrl+n":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-10)
	case "pgdown":
		m.moveCursor(10)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.moveCursor(len(m.results))
	case "tab", "/":
		return m, m.input.Focus()
	case "enter":
		return m, m.open()
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if len(m.results) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.results) {
		m.cursor = len(m.results) - 1
	}
}

// open drills into whatever the selected result points at.
func (m *Model) open() tea.Cmd {
	result, ok := m.SelectedResult()
	if !ok {
		return nil
	}

	switch result.Kind {
	case KindCompetition:
		if m.deps.Nav.League == nil {
			return nil
		}
		return core.Push(m.deps.Nav.League(m.deps, result.Competition.ID, ""))

	case KindTeam:
		if m.deps.Nav.Team == nil {
			return nil
		}
		// Browse has no competition/season context to pass on; the Team
		// screen defaults to the most recent season of the club's own
		// league.
		return core.Push(m.deps.Nav.Team(m.deps, result.Team, "", ""))

	default:
		if m.deps.Nav.Player == nil {
			return nil
		}
		return core.Push(m.deps.Nav.Player(m.deps, result.Player, ""))
	}
}
