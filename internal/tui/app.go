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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
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
}

var _ tea.Model = (*App)(nil)

// NewApp constructs the root model with initial as the bottom of the
// navigation stack.
func NewApp(deps core.Deps, initial core.Screen) *App {
	return &App{
		deps:   deps,
		stack:  []core.Screen{initial},
		width:  100,
		height: 30,
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
	}
}

// NewLeagueScreen constructs the League screen, for callers (the CLI) that
// want to start the app on a specific competition.
func NewLeagueScreen(deps core.Deps, competitionID string) core.Screen {
	return league.New(deps, competitionID)
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

// chromeHeight is the number of lines App reserves for the breadcrumb and
// status line.
const chromeHeight = 3

// View implements tea.Model.
func (a *App) View() string {
	var b strings.Builder

	if crumb := a.breadcrumb(); crumb != "" {
		b.WriteString(crumb)
		b.WriteString("\n\n")
	}

	b.WriteString(a.current().View())

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

func (a *App) statusLine() string {
	if a.status != "" {
		return a.deps.Theme.Status.Render(a.status)
	}
	if a.err != nil {
		return a.deps.Theme.Error.Render("error: " + a.err.Error())
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
