// Package core is the contract every TUI screen shares, kept apart from the
// root app so screens never import it. Screens reach data only through
// Deps.Repos (docs/architecture.md, boundary 3).
package core

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Deps is everything a screen is constructed with.
type Deps struct {
	Ctx   context.Context
	Repos storage.Repositories
	Theme styles.Theme
	// Nav is injected so screen packages never import each other.
	Nav Navigator
	// Refresh runs one RefreshRequest. Nil (tests, mostly) makes the app
	// tell the user to use the CLI.
	Refresh Refresher
	// Catalog is every competition the host can fetch.
	Catalog []CatalogEntry
	// Appearance is shared by every screen so a toggle reaches all of
	// them; nil means weak.
	Appearance *styles.Appearance
	// SaveTheme persists a runtime theme choice; nil means session-only.
	SaveTheme func(styles.Mode) error
	// Favorites is shared by every screen so a toggle shows everywhere.
	Favorites *Favorites
	// ClubDue reports whether a club's recent games need fetching, without
	// fetching. Nil means never.
	ClubDue func(club models.Team) (why string, due bool)
	// ProviderPause is when a rate-limit pause ends; zero means none, nil
	// means the host can't tell. The TUI never names the source.
	ProviderPause func() (until time.Time)
	// Players searches the source beyond the cache; nil means local only.
	Players PlayerSearcher
	// Version is shown on the splash; empty leaves it off.
	Version string
	// Log records background downloads; nil records nothing.
	Log *Log
}

// PlayerSearcher mirrors the provider contract of the same name, so the TUI
// needn't import it. PlayerCareer is newest first.
type PlayerSearcher interface {
	SearchPlayers(ctx context.Context, query string) ([]models.FoundPlayer, error)
	PlayerCareer(ctx context.Context, id string) ([]models.CareerSeason, error)
}

// ThemeMode is the current theme mode, weak when none was configured.
func (d Deps) ThemeMode() styles.Mode {
	if d.Appearance == nil {
		return styles.Weak
	}
	return d.Appearance.Mode
}

// ThemeChangedMsg is broadcast to every screen on a theme switch. Every
// screen must adopt Theme, or a painted background sits under text styled
// for the terminal's; club screens rebuild their theme in Mode.
type ThemeChangedMsg struct {
	Mode  styles.Mode
	Theme styles.Theme
}

// ThemedScreen is a screen with its own theme; the app draws its chrome in
// it too.
type ThemedScreen interface {
	ScreenTheme() styles.Theme
}

// Navigator is the set of screen constructors for drill-down. Screens treat
// a nil field as a no-op.
type Navigator struct {
	// League opens at the latest cached season when seasonID is empty.
	League func(deps Deps, competitionID, seasonID string) Screen
	Team   func(deps Deps, team models.Team, competitionID, seasonID string) Screen
	Nation func(deps Deps, team models.Team) Screen
	// Games opens a club's last games in every competition.
	Games func(deps Deps, team models.Team) Screen
	// Match opens one game in full, seen from teamID's side ("" for
	// neither).
	Match   func(deps Deps, match models.Match, teamID string) Screen
	Player  func(deps Deps, player models.Player, teamID string) Screen
	Browse  func(deps Deps, query string) Screen
	Catalog func(deps Deps) Screen
}

// Screen is tea.Model with Update returning a Screen, so the stack needs no
// type assertions.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
	// Title is shown in the breadcrumb.
	Title() string
	// Keys drives the footer, so documented keys can't drift from handled
	// ones.
	Keys() KeyMap
}

// KeyMap is a screen's footer bindings and the full set behind "?".
type KeyMap struct {
	Short []key.Binding
	// Full is grouped into columns; empty means Short.
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

// PushMsg asks the root model to push a screen; "back" is always a pop.
type PushMsg struct{ Screen Screen }

// PopMsg asks the root model to pop; popping the last screen quits.
type PopMsg struct{}

// ErrMsg reports a failure to surface to the user.
type ErrMsg struct{ Err error }

// ClickMsg is a click on a components.Zone. The wheel arrives as up/down
// keys instead.
type ClickMsg struct{ ID string }

// ClickedIndex reads a click on a region marked prefix+":"+index, such as
// a table row (components.TableOpts.Zone).
func (c ClickMsg) ClickedIndex(prefix string) (int, bool) {
	rest, ok := strings.CutPrefix(c.ID, prefix+":")
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(rest)
	return i, err == nil
}

// StatusMsg sets a transient one-line footer message.
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

// TextEntry is a screen capturing free text; while true, bare letters
// aren't global shortcuts.
type TextEntry interface {
	CapturingText() bool
}

// Escaper lets esc unwind screen state before navigating back. Escape
// reports whether it consumed the key.
type Escaper interface {
	Escape() (handled bool, cmd tea.Cmd)
}

// Searcher hosts the search box; the search key returns to it rather than
// stacking a new one.
type Searcher interface {
	FocusSearch() tea.Cmd
}
