// Package core defines the rendering contract: the small set of types every
// TUI screen shares. It exists as its own package so screen packages
// (internal/tui/screens/*) can depend on the contract without depending on
// the root app model that hosts them — the root imports the screens, never
// the other way around.
//
// Screens reach data exclusively through Deps.Repos, a
// storage.Repositories. No screen imports internal/data/provider or
// internal/storage/sqlite: per docs/architecture.md's boundary 3, the TUI
// consumes only the backend contract's query methods.
package core

import (
	"context"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Deps is everything a screen is constructed with. Passing one value
// rather than a growing argument list keeps screen constructors stable as
// new shared concerns (theme, clock, feature flags) appear.
type Deps struct {
	Ctx   context.Context
	Repos storage.Repositories
	Theme styles.Theme
	// Nav constructs the screens this screen can drill into. Injecting
	// constructors instead of importing sibling screen packages directly
	// keeps navigation acyclic: League can open Team and Team can open
	// League without the two packages importing each other.
	Nav Navigator
	// Refresh runs a data refresh for one competition. Nil means the
	// host didn't wire one up (tests, mostly); the root app then tells
	// the user to use the CLI instead of failing silently.
	Refresh Refresher
	// Catalog is every competition the host can fetch, for the league
	// catalog screen. Empty means the host offered none (tests, mostly).
	Catalog []CatalogEntry
	// Appearance is the user's theme choice. It is a pointer shared by
	// every screen so a runtime toggle reaches all of them; nil means
	// the default (weak).
	Appearance *styles.Appearance
	// SaveTheme records a theme chosen at runtime as the user's default.
	// Nil means the host can't persist settings, and the toggle only
	// lasts for the session.
	SaveTheme func(styles.Mode) error
	// Favorites are the user's starred entities, shared by every screen
	// so a toggle shows everywhere at once.
	Favorites *Favorites
}

// ThemeMode is the current theme mode, weak when none was configured.
func (d Deps) ThemeMode() styles.Mode {
	if d.Appearance == nil {
		return styles.Weak
	}
	return d.Appearance.Mode
}

// ThemeChangedMsg is broadcast to every screen when the user switches
// theme mode. Theme is the new theme for content with no club colors of
// its own; every screen adopts it (it draws its own content, so a painted
// background would otherwise sit under text styled for the terminal's),
// and a screen showing a club rebuilds that club's theme in Mode.
type ThemeChangedMsg struct {
	Mode  styles.Mode
	Theme styles.Theme
}

// ThemedScreen is implemented by screens that render in a theme of their
// own (a club's colors). The root app draws its chrome — breadcrumb,
// footer, status line — in that theme too, and paints the whole frame in
// its background when it has one.
type ThemedScreen interface {
	ScreenTheme() styles.Theme
}

// Navigator is the set of screen constructors available for drill-down.
// The root app (internal/tui) populates it once at startup; tests fill in
// only the fields the screen under test actually uses, and screens treat
// a nil field as a drill-down that does nothing.
type Navigator struct {
	// League opens a competition's standings at seasonID, or at its most
	// recent cached season when seasonID is empty.
	League func(deps Deps, competitionID, seasonID string) Screen
	Team   func(deps Deps, team models.Team, competitionID, seasonID string) Screen
	Player func(deps Deps, player models.Player, teamID string) Screen
	Browse func(deps Deps, query string) Screen
	// Catalog opens the screen for choosing leagues and seasons to fetch.
	Catalog func(deps Deps) Screen
}

// Screen is one view in the navigation stack. It mirrors tea.Model except
// that Update returns a Screen, so the root model can keep a stack of them
// without type assertions on every keystroke.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
	// Title is shown in the app's breadcrumb bar.
	Title() string
	// Keys are the bindings this screen responds to. The root app renders
	// the footer from them, so a screen never writes its own help text and
	// the documented keys cannot drift from the handled ones.
	Keys() KeyMap
}

// KeyMap is a screen's bindings, split into the few worth a permanent
// footer and the full set behind "?".
type KeyMap struct {
	// Short is the always-visible footer line. Keep it to the handful a
	// user needs to get around.
	Short []key.Binding
	// Full is the expanded help, grouped into columns. When empty, Short
	// is used for both.
	Full [][]key.Binding
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding { return k.Short }

// FullHelp implements help.KeyMap.
func (k KeyMap) FullHelp() [][]key.Binding {
	if len(k.Full) == 0 {
		return [][]key.Binding{k.Short}
	}
	return k.Full
}

// PushMsg asks the root model to push a new screen onto the stack. A screen
// drills down by returning Push(next) rather than swapping itself out, so
// "back" is always just popping — no screen has to remember where it came
// from or reconstruct its parent's state.
type PushMsg struct{ Screen Screen }

// PopMsg asks the root model to pop the current screen. Popping the last
// screen quits the app.
type PopMsg struct{}

// ErrMsg reports a failure that should be surfaced to the user rather than
// crashing the program — typically a repository query that failed.
type ErrMsg struct{ Err error }

// StatusMsg sets a transient one-line message in the footer, for feedback
// that doesn't warrant a whole screen (e.g. "no roster cached for 2019").
type StatusMsg struct{ Text string }

// Push returns a command that pushes s onto the navigation stack.
func Push(s Screen) tea.Cmd {
	return func() tea.Msg { return PushMsg{Screen: s} }
}

// Pop returns a command that pops the current screen.
func Pop() tea.Cmd {
	return func() tea.Msg { return PopMsg{} }
}

// Status returns a command that sets the footer status line.
func Status(text string) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: text} }
}

// TextEntry is implemented by screens that capture free-text input (the
// Browse search box). While such a screen reports true, the root app stops
// interpreting bare letter keys as shortcuts, so typing "q" enters a "q"
// instead of quitting the program.
type TextEntry interface {
	CapturingText() bool
}

// Escaper is implemented by screens with state that esc should unwind
// before it navigates back — a search query to clear, say. Escape reports
// whether the screen consumed the key.
type Escaper interface {
	Escape() (handled bool, cmd tea.Cmd)
}

// Searcher is implemented by the screen that hosts the search box. The
// root app's search key returns to it, rather than stacking up a new
// search screen every time it is pressed.
type Searcher interface {
	FocusSearch() tea.Cmd
}
