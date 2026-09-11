// Package tui is the root of footui's terminal interface: it owns the
// navigation stack, the global key bindings, and the chrome (breadcrumb,
// status line) drawn around whichever screen is on top.
//
// It is the only package that imports the individual screen packages —
// screens themselves navigate through core.Navigator constructors handed
// to them in core.Deps, so no screen imports another.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/screens/browse"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// App is the root Bubble Tea model.
type App struct {
	deps  core.Deps
	stack []core.Screen

	width  int
	height int

	status string
	err    error

	// refreshing is the status of an in-flight refresh, shown in place of
	// the status line until it finishes; empty when none is running.
	// Unlike status it survives keystrokes, so the user can keep
	// navigating while it runs without losing sight of it.
	refreshing        string
	lastRefreshTarget string

	help     help.Model
	showFull bool
}

var _ tea.Model = (*App)(nil)

// NewApp constructs the root model with initial as the bottom of the
// navigation stack.
func NewApp(deps core.Deps, initial core.Screen) *App {
	h := help.New()
	h.ShowAll = false
	return &App{
		deps:   deps,
		stack:  []core.Screen{initial},
		width:  100,
		height: 30,
		help:   h,
	}
}

// NewDeps builds the dependency bundle every screen is constructed with,
// including the Navigator that wires screen-to-screen drill-down.
func NewDeps(ctx context.Context, repos storage.Repositories, theme styles.Theme) core.Deps {
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: theme}
	deps.Nav = navigator()
	return deps
}

// navigator maps each drill-down route to the screen package that
// implements it. Routes not yet built are left nil; screens check for nil
// and show a status message rather than crashing.
func navigator() core.Navigator {
	return core.Navigator{
		League: func(deps core.Deps, competitionID string) core.Screen {
			return league.New(deps, competitionID)
		},
		Team: func(deps core.Deps, t models.Team, competitionID, seasonID string) core.Screen {
			return team.New(deps, t, competitionID, seasonID)
		},
		Player: func(deps core.Deps, p models.Player, teamID string) core.Screen {
			return player.New(deps, p, teamID)
		},
		Browse: func(deps core.Deps, query string) core.Screen {
			return browse.New(deps, query)
		},
	}
}

// NewLeagueScreen constructs the League screen, for callers (the CLI) that
// want to start the app on a specific competition.
func NewLeagueScreen(deps core.Deps, competitionID string) core.Screen {
	return league.New(deps, competitionID)
}

// NewTeamScreen constructs the Team screen, for `footy team <name>`.
func NewTeamScreen(deps core.Deps, t models.Team) core.Screen {
	return team.New(deps, t, "", "")
}

// NewPlayerScreen constructs the Player screen, for `footy player <name>`.
func NewPlayerScreen(deps core.Deps, p models.Player) core.Screen {
	return player.New(deps, p, "")
}

// NewBrowseScreen constructs the Browse screen, optionally pre-filled with
// a search query. It is what bare `footy` opens, and where an ambiguous
// `footy team <name>` lands.
func NewBrowseScreen(deps core.Deps, query string) core.Screen {
	return browse.New(deps, query)
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return a.current().Init()
}

func (a *App) current() core.Screen {
	return a.stack[len(a.stack)-1]
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// A terminal that can't report its size sends zeroes (a pty with
		// no winsize set, some CI runners). Taking those literally
		// collapses every column to its minimum and truncates club names
		// to nothing, so the sensible default is kept instead.
		if msg.Width <= 0 || msg.Height <= 0 {
			return a, nil
		}
		a.width, a.height = msg.Width, msg.Height
		// Every screen in the stack is resized, not just the visible one,
		// so a screen the user pops back to isn't laid out for the old
		// terminal size.
		var cmds []tea.Cmd
		for i, s := range a.stack {
			updated, cmd := s.Update(a.screenSize())
			a.stack[i] = updated
			cmds = append(cmds, cmd)
		}
		return a, tea.Batch(cmds...)

	case core.PushMsg:
		if msg.Screen == nil {
			return a, nil
		}
		a.status = ""
		a.stack = append(a.stack, msg.Screen)
		sized, sizeCmd := msg.Screen.Update(a.screenSize())
		a.stack[len(a.stack)-1] = sized
		return a, tea.Batch(sizeCmd, sized.Init())

	case core.PopMsg:
		return a.pop()

	case core.QuitMsg:
		return a, tea.Quit

	case core.StatusMsg:
		a.status = msg.Text
		return a, nil

	case core.ErrMsg:
		// Errors are shown by the screen that requested the data, since it
		// knows the context; the app keeps a copy only so a screen that
		// ignores ErrMsg doesn't swallow the failure silently.
		a.err = msg.Err

	case refreshMsg:
		return a, a.handleRefreshEvent(msg)
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if handled, model, cmd := a.handleGlobalKey(key); handled {
			return model, cmd
		}
		// Any keystroke that reaches a screen clears a stale status line.
		a.status = ""
	}

	updated, cmd := a.current().Update(msg)
	a.stack[len(a.stack)-1] = updated
	return a, cmd
}

// handleGlobalKey intercepts the bindings that work on every screen. It
// reports whether it consumed the key.
func (a *App) handleGlobalKey(key tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return true, a, tea.Quit
	case "esc":
		model, cmd := a.pop()
		return true, model, cmd
	case "q":
		// A screen capturing text needs the literal character.
		if entry, ok := a.current().(core.TextEntry); ok && entry.CapturingText() {
			return false, a, nil
		}
		return true, a, tea.Quit
	case "?":
		if entry, ok := a.current().(core.TextEntry); ok && entry.CapturingText() {
			return false, a, nil
		}
		a.showFull = !a.showFull
		a.help.ShowAll = a.showFull
		return true, a, nil
	case "r":
		if entry, ok := a.current().(core.TextEntry); ok && entry.CapturingText() {
			return false, a, nil
		}
		return true, a, a.startRefresh()
	case "ctrl+r":
		return true, a, a.startRefresh()
	}
	return false, a, nil
}

// pop removes the top screen, quitting when the stack would be emptied.
func (a *App) pop() (tea.Model, tea.Cmd) {
	if len(a.stack) <= 1 {
		return a, tea.Quit
	}
	a.stack = a.stack[:len(a.stack)-1]
	a.status = ""
	a.err = nil
	return a, nil
}

// screenSize is the window size handed to screens: the full terminal minus
// the chrome the app draws itself.
func (a *App) screenSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: a.width, Height: a.height - chromeHeight}
}

// chromeHeight is the number of lines App reserves for the breadcrumb,
// footer, and status line.
const chromeHeight = 5

// View implements tea.Model.
func (a *App) View() string {
	var b strings.Builder

	if crumb := a.breadcrumb(); crumb != "" {
		b.WriteString(crumb)
		b.WriteString("\n\n")
	}

	b.WriteString(a.current().View())

	b.WriteString("\n\n")
	b.WriteString(a.footer())

	if line := a.statusLine(); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
	}

	return b.String()
}

func (a *App) breadcrumb() string {
	if len(a.stack) <= 1 {
		return ""
	}
	titles := make([]string, 0, len(a.stack))
	for _, s := range a.stack {
		titles = append(titles, s.Title())
	}
	return a.deps.Theme.Breadcrumb.Render(strings.Join(titles, " › "))
}

// footer renders the current screen's key bindings. Screens declare
// bindings rather than help strings, so what the footer promises is always
// what the screen actually handles.
func (a *App) footer() string {
	// The global bindings are the app's, not the screen's, so they're
	// appended here rather than repeated in every screen's KeyMap — and
	// to both views, or the expanded help would omit how to get out of it.
	globals := []key.Binding{keys.Help, keys.Back, keys.Quit}
	if entry, ok := a.current().(core.TextEntry); ok && entry.CapturingText() {
		// "?" is also swallowed by the text box, so neither it nor "q" is
		// offered here.
		globals = []key.Binding{keys.Back, keys.QuitFromTextEntry}
	}
	if refresh, ok := a.refreshBinding(); ok {
		globals = append([]key.Binding{refresh}, globals...)
	}

	km := a.current().Keys()
	km.Full = append(append([][]key.Binding{}, km.FullHelp()...), globals)
	km.Short = append(append([]key.Binding{}, km.Short...), globals...)

	a.help.Width = a.width
	a.help.Styles.ShortKey = a.deps.Theme.Accent
	a.help.Styles.ShortDesc = a.deps.Theme.Help
	a.help.Styles.ShortSeparator = a.deps.Theme.Help
	a.help.Styles.FullKey = a.deps.Theme.Accent
	a.help.Styles.FullDesc = a.deps.Theme.Help
	a.help.Styles.FullSeparator = a.deps.Theme.Help

	return a.help.View(km)
}

// statusLine is the single row beneath the footer. Messages are cut to the
// terminal width: a wrapped status would push the chrome off the bottom of
// the screen.
func (a *App) statusLine() string {
	switch {
	case a.refreshing != "":
		return a.deps.Theme.Status.Render(components.Truncate(a.refreshing, a.width))
	case a.status != "":
		return a.deps.Theme.Status.Render(components.Truncate(a.status, a.width))
	case a.err != nil:
		return a.deps.Theme.Error.Render(components.Truncate("error: "+a.err.Error(), a.width))
	}
	return ""
}

// Run starts the program on the given screen and blocks until the user
// quits.
func Run(deps core.Deps, initial core.Screen) error {
	p := tea.NewProgram(NewApp(deps, initial), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
