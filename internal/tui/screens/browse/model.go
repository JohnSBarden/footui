// Package browse is the entry screen: search over the local cache. The CLI
// also opens it pre-filled to disambiguate a name like "United".
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
	// KindFound is an uncached player; opening one downloads their seasons.
	KindFound
)

// Result is one row in the results list; Kind selects the meaningful field.
type Result struct {
	Kind        Kind
	Competition models.Competition
	Team        models.Team
	Player      models.Player
	Found       models.FoundPlayer
}

// Name is the result's primary label.
func (r Result) Name() string {
	switch r.Kind {
	case KindCompetition:
		return r.Competition.Name
	case KindTeam:
		return r.Team.Name
	case KindFound:
		return r.Found.Name
	default:
		return r.Player.Name
	}
}

// Model is the Browse screen.
type Model struct {
	deps  core.Deps
	input textinput.Model

	// results is local, then found minus names already cached.
	results          []Result
	local            []Result
	found            []Result
	searchingFurther bool
	// awaiting is the found player downloading; the cursor moves to them
	// when they land.
	awaiting *models.FoundPlayer
	// awaitStarted is set once the downloads have shown on the queue, so
	// its draining means they're done.
	awaitStarted bool
	colors       map[string]models.TeamColors
	cursor       int

	// query guards against a slow query overwriting a newer one's results.
	query string
	// searched: later re-runs (after a refresh) keep the cursor.
	searched bool
	loading  bool
	err      error

	width  int
	height int
}

// New constructs the Browse screen, pre-filled with query (may be empty).
func New(deps core.Deps, query string) *Model {
	in := textinput.New()
	in.Placeholder = "clubs, nations, players, leagues"
	in.Prompt = "> "
	in.PromptStyle = deps.Theme.Accent.Bold(true)
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

// Keys implements core.Screen. A focused search box advertises only the
// bindings that survive text entry.
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

// Escape implements core.Escaper: results → box → clear query → back.
func (m *Model) Escape() (bool, tea.Cmd) {
	if !m.input.Focused() {
		return true, m.input.Focus()
	}
	if m.input.Value() == "" {
		return false, nil
	}
	m.input.SetValue("")
	m.newQuery("")
	return true, m.search("")
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.search(m.query))
}

// defaultCompetition is fetched on a first-run refresh; keep in sync with
// the empty-state hint.
const defaultCompetition = "eng-premier-league"

// RefreshTarget implements core.Refreshable: the highlighted competition,
// or the default one on an empty cache. Teams and players have none.
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

// FavoriteTarget implements core.Favoritable: the highlighted result.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	r, ok := m.SelectedResult()
	if !ok || r.Kind == KindFound {
		return core.FavoriteTarget{}, false
	}
	kind, id := r.favoriteKey()
	return core.FavoriteTarget{Kind: kind, ID: id, Name: r.Name()}, true
}

func (r Result) favoriteKey() (models.FavoriteKind, string) {
	switch r.Kind {
	case KindCompetition:
		return models.FavoriteCompetition, r.Competition.ID
	case KindTeam:
		return models.FavoriteTeam, r.Team.ID
	default:
		return models.FavoritePlayer, r.Player.ID
	}
}

// firstRun reports whether the cache is empty.
func (m *Model) firstRun() bool {
	return !m.loading && len(m.results) == 0 && strings.TrimSpace(m.input.Value()) == ""
}

// CapturingText implements core.TextEntry.
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
	colors  map[string]models.TeamColors
}
