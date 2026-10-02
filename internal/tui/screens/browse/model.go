// Package browse is footui's entry screen: a search box over everything
// cached locally — competitions, teams, and players — plus the list of
// competitions to start from when the box is empty.
//
// It doubles as the disambiguation screen: `footy team "United"` matches
// several clubs, so the CLI opens Browse pre-filled with the query rather
// than making the user guess an exact name.
package browse

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"strings"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// Kind is what a search result points at.
type Kind int

const (
	KindCompetition Kind = iota
	KindTeam
	KindPlayer
)

// Result is one row in the results list. Exactly one of the embedded
// values is meaningful, selected by Kind.
type Result struct {
	Kind        Kind
	Competition models.Competition
	Team        models.Team
	Player      models.Player
}

// Name is the result's primary label.
func (r Result) Name() string {
	switch r.Kind {
	case KindCompetition:
		return r.Competition.Name
	case KindTeam:
		return r.Team.Name
	default:
		return r.Player.Name
	}
}

// Model is the Browse screen.
type Model struct {
	deps  core.Deps
	input textinput.Model

	results []Result
	cursor  int

	// query is the search the current results belong to, so a slow query's
	// results can't overwrite a newer one's.
	query   string
	loading bool
	err     error

	width  int
	height int
}

// New constructs the Browse screen, pre-filled with query (which may be
// empty).
func New(deps core.Deps, query string) *Model {
	in := textinput.New()
	in.Placeholder = "search competitions, teams, players…"
	in.Prompt = "› "
	in.SetValue(query)
	in.CursorEnd()
	in.Focus()

	return &Model{
		deps:    deps,
		input:   in,
		query:   query,
		loading: true,
		width:   100,
		height:  30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string { return "Browse" }

// Keys implements core.Screen. While the search box has focus it owns
// most keystrokes, so only the bindings that survive text entry are
// advertised; tab hands the keyboard to the results list.
func (m *Model) Keys() core.KeyMap {
	if m.input.Focused() {
		short := []key.Binding{keys.MoveInTextEntry, keys.Open, keys.ToResults}
		if m.input.Value() != "" {
			short = append(short, keys.ClearSearch)
		}
		return core.KeyMap{Short: short}
	}
	return core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Open, keys.Find},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End, keys.Open, keys.Find},
		},
	}
}

// FocusSearch implements core.Searcher.
func (m *Model) FocusSearch() tea.Cmd {
	return m.input.Focus()
}

// Escape implements core.Escaper. esc unwinds the search before it
// leaves the screen: from the results it returns to the box, and in the
// box it clears a query. Only esc on an empty box goes back.
func (m *Model) Escape() (bool, tea.Cmd) {
	if !m.input.Focused() {
		return true, m.input.Focus()
	}
	if m.input.Value() == "" {
		return false, nil
	}
	m.input.SetValue("")
	m.query = ""
	m.loading = true
	return true, m.search("")
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.search(m.query))
}

// defaultCompetition is what a first-run refresh fetches: the same one the
// empty state has always told the user to fetch by hand.
const defaultCompetition = "eng-premier-league"

// RefreshTarget implements core.Refreshable. With a competition result
// highlighted, that competition is refreshed. With nothing cached at all
// there is nothing to pick from, so the default competition is; a
// highlighted team or player has no single competition to refresh.
func (m *Model) RefreshTarget() (string, bool) {
	if r, ok := m.SelectedResult(); ok {
		if r.Kind == KindCompetition {
			return r.Competition.ID, true
		}
		return "", false
	}
	if m.firstRun() {
		return defaultCompetition, true
	}
	return "", false
}

// firstRun reports whether the cache is empty: a blank query returned
// nothing, and even the loading spinner has settled.
func (m *Model) firstRun() bool {
	return !m.loading && len(m.results) == 0 && strings.TrimSpace(m.input.Value()) == ""
}

// CapturingText implements core.TextEntry: while the search box has focus,
// bare letters must reach it rather than triggering global shortcuts.
func (m *Model) CapturingText() bool { return m.input.Focused() }

// SelectedResult returns the result under the cursor.
func (m *Model) SelectedResult() (Result, bool) {
	if m.cursor < 0 || m.cursor >= len(m.results) {
		return Result{}, false
	}
	return m.results[m.cursor], true
}

type resultsMsg struct {
	query   string
	results []Result
}
