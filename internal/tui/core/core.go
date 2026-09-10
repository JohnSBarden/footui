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
}

// Navigator is the set of screen constructors available for drill-down.
// The root app (internal/tui) populates it once at startup; tests fill in
// only the fields the screen under test actually uses, and a nil field
// simply means that route isn't wired yet.
type Navigator struct {
	League func(deps Deps, competitionID string) Screen
	Team   func(deps Deps, team models.Team, competitionID, seasonID string) Screen
	Player func(deps Deps, player models.Player, teamID string) Screen
	Browse func(deps Deps, query string) Screen
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

// Fail returns a command that surfaces err in the UI. A nil err produces no
// message, so callers can pass a query error through unconditionally.
func Fail(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return func() tea.Msg { return ErrMsg{Err: err} }
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

// QuitMsg asks the root model to exit the program.
type QuitMsg struct{}

// Quit returns a command that exits the program.
func Quit() tea.Cmd {
	return func() tea.Msg { return QuitMsg{} }
}
