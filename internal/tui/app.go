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
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/kickabout"
	"github.com/JohnSBarden/footui/internal/tui/screens/browse"
	"github.com/JohnSBarden/footui/internal/tui/screens/catalog"
	"github.com/JohnSBarden/footui/internal/tui/screens/dashboard"
	"github.com/JohnSBarden/footui/internal/tui/screens/games"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
	"github.com/JohnSBarden/footui/internal/tui/screens/nation"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// App is the root Bubble Tea model.
type App struct {
	deps  core.Deps
	stack []core.Screen
	// ids parallels stack with a unique id per entry, so a message
	// produced by a screen beneath the top can be routed back to it.
	ids    []int
	nextID int

	width  int
	height int

	status string
	err    error

	// refreshing is the status of an in-flight refresh, shown in place of
	// the status line until it finishes; empty when none is running.
	// Unlike status it survives keystrokes, so the user can keep
	// navigating while it runs without losing sight of it.
	refreshing string
	// active is the refresh running now, nil when idle; queue is what
	// waits behind it. batch collects outcomes until the queue drains.
	active *core.RefreshRequest
	queue  []core.RefreshRequest
	batch  refreshBatch
	// stopActive cancels the running refresh; stopping is set once the
	// user has asked it to stop, until it reports back.
	stopActive context.CancelFunc
	stopping   bool
	// startup is refresh work to queue as soon as the app starts — the
	// stale current seasons the host found when it opened the database.
	startup []core.RefreshRequest

	help     help.Model
	showFull bool

	// splash is the launch screen, nil once it's gone.
	splash *splashState

	// kick is the kickabout being played over the screen, nil when none
	// is. While it runs it takes the keys; every other message still
	// reaches the stack, so the screen is current when the game ends.
	kick *kickabout.Model
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
		ids:    []int{0},
		nextID: 1,
		width:  100,
		height: 30,
		help:   h,
	}
}

// NewDeps builds the dependency bundle every screen is constructed with,
// including the Navigator that wires screen-to-screen drill-down.
func NewDeps(ctx context.Context, repos storage.Repositories, theme styles.Theme) core.Deps {
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: theme, Favorites: core.NewFavorites(nil)}
	deps.Nav = navigator()
	return deps
}

// navigator maps each drill-down route to the screen package that
// implements it.
func navigator() core.Navigator {
	return core.Navigator{
		League: func(deps core.Deps, competitionID, seasonID string) core.Screen {
			return league.NewAt(deps, competitionID, seasonID)
		},
		Team: func(deps core.Deps, t models.Team, competitionID, seasonID string) core.Screen {
			// Every route to a team — a table row, a search result, a
			// card — lands on the nation screen for a national team.
			if t.National {
				return nation.New(deps, t)
			}
			return team.New(deps, t, competitionID, seasonID)
		},
		Nation: func(deps core.Deps, t models.Team) core.Screen {
			return nation.New(deps, t)
		},
		Games: func(deps core.Deps, t models.Team) core.Screen {
			return games.New(deps, t)
		},
		Player: func(deps core.Deps, p models.Player, teamID string) core.Screen {
			return player.New(deps, p, teamID)
		},
		Browse: func(deps core.Deps, query string) core.Screen {
			return browse.New(deps, query)
		},
		Catalog: func(deps core.Deps) core.Screen {
			return catalog.New(deps)
		},
	}
}

// NewLeagueScreen constructs the League screen, for callers (the CLI) that
// want to start the app on a specific competition.
func NewLeagueScreen(deps core.Deps, competitionID string) core.Screen {
	return league.New(deps, competitionID)
}

// NewTeamScreen constructs the Team screen, for `footui team <name>` — or
// the nation screen, for a national team.
func NewTeamScreen(deps core.Deps, t models.Team) core.Screen {
	if t.National {
		return nation.New(deps, t)
	}
	return team.New(deps, t, "", "")
}

// NewPlayerScreen constructs the Player screen, for `footui player <name>`.
func NewPlayerScreen(deps core.Deps, p models.Player) core.Screen {
	return player.New(deps, p, "")
}

// NewBrowseScreen constructs the Browse screen, optionally pre-filled with
// a search query. It is where an ambiguous `footui team <name>` lands, and
// what bare `footui` opens before anything is favourited.
func NewBrowseScreen(deps core.Deps, query string) core.Screen {
	return browse.New(deps, query)
}

// NewHomeScreen is what bare `footui` opens: the dashboard once anything is
// favourited, Browse until then.
func NewHomeScreen(deps core.Deps) core.Screen {
	if deps.Favorites.Len() > 0 {
		return dashboard.New(deps)
	}
	return browse.New(deps, "")
}

// QueueOnStart arranges for reqs to be refreshed as soon as the app
// starts. It is how a stale current season gets brought up to date on
// open without the user asking, while they carry on browsing.
func (a *App) QueueOnStart(reqs ...core.RefreshRequest) {
	a.startup = append(a.startup, reqs...)
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	init := a.current().Init()
	if a.splash != nil {
		init = tea.Batch(init, splashTimer())
	}
	if len(a.startup) == 0 || a.deps.Refresh == nil {
		return init
	}
	reqs := a.startup
	a.startup = nil
	return tea.Batch(init, a.enqueue(reqs...))
}

func (a *App) current() core.Screen {
	return a.stack[len(a.stack)-1]
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.splash != nil {
		if key, ok := msg.(tea.KeyMsg); ok {
			// Any key skips the splash; it isn't passed on, since the
			// user pressed it at the splash, not at the screen.
			a.splash = nil
			if key.String() == "ctrl+c" {
				return a, tea.Quit
			}
			return a, nil
		}
	}
	if a.kick != nil {
		if handled, cmd := a.updateKick(msg); handled {
			return a, cmd
		}
	}
	switch msg := msg.(type) {
	case splashDoneMsg:
		a.splashTimedOut()
		return a, nil

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
			cmds = append(cmds, a.routeFrom(i, cmd))
		}
		if a.kick != nil {
			// The pitch is the screen as it now looks, so it's laid out
			// again from scratch.
			cmds = append(cmds, a.startKick())
		}
		return a, tea.Batch(cmds...)

	case core.PushMsg:
		if msg.Screen == nil {
			return a, nil
		}
		a.status = ""
		a.stack = append(a.stack, msg.Screen)
		a.ids = append(a.ids, a.nextID)
		a.nextID++
		sized, sizeCmd := msg.Screen.Update(a.screenSize())
		// A screen pushed mid-refresh learns what's in flight now rather
		// than at the next queue change.
		sized, queueCmd := sized.Update(core.RefreshQueueMsg{Active: a.active, Pending: slices.Clone(a.queue)})
		a.stack[len(a.stack)-1] = sized
		return a, tea.Batch(sizeCmd, queueCmd, sized.Init())

	case core.PopMsg:
		return a.pop()

	case routedMsg:
		return a, a.deliver(msg)

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

	case core.RefreshRequestMsg:
		return a, a.enqueue(msg.Requests...)

	case core.CancelRefreshMsg:
		return a, a.cancelRefresh()

	case core.FavoriteToggledMsg:
		return a, a.favoriteToggled(msg)
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
		if esc, ok := a.current().(core.Escaper); ok {
			if handled, cmd := esc.Escape(); handled {
				return true, a, cmd
			}
		}
		model, cmd := a.pop()
		return true, model, cmd
	case "/":
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, a.openSearch()
	case "q":
		// A screen capturing text needs the literal character.
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, tea.Quit
	case "?":
		if a.capturingText() {
			return false, a, nil
		}
		a.showFull = !a.showFull
		a.help.ShowAll = a.showFull
		return true, a, nil
	case "r":
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, a.startRefresh()
	case "ctrl+r":
		return true, a, a.startRefresh()
	case "d":
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, a.openCatalog()
	case "ctrl+g":
		return true, a, a.openCatalog()
	case "t":
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, a.toggleTheme()
	case "ctrl+t":
		return true, a, a.toggleTheme()
	case "f":
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, a.toggleFavorite()
	case "ctrl+f":
		return true, a, a.toggleFavorite()
	case "ctrl+k":
		return true, a, a.startKick()
	}
	return false, a, nil
}

// startKick turns the screen as it looks now into a pitch.
func (a *App) startKick() tea.Cmd {
	k := kickabout.New(a.baseView(), a.width, a.height, a.theme())
	a.kick = &k
	return k.Init()
}

// updateKick gives the game its keys and ticks, reporting whether it
// consumed msg. esc ends the game; ctrl+c still quits.
func (a *App) updateKick(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return true, tea.Quit
		}
		k, cmd := a.kick.Update(msg)
		if k.Done() {
			a.kick = nil
			return true, nil
		}
		a.kick = &k
		return true, cmd
	case kickabout.TickMsg:
		k, cmd := a.kick.Update(msg)
		a.kick = &k
		return true, cmd
	}
	return false, nil
}

// theme is what the chrome is drawn in: the visible screen's own theme
// when it has one (a club's colors), the app's otherwise.
func (a *App) theme() styles.Theme {
	if themed, ok := a.current().(core.ThemedScreen); ok {
		return themed.ScreenTheme()
	}
	return a.deps.Theme
}

// toggleTheme switches between weak and strong club themes.
func (a *App) toggleTheme() tea.Cmd {
	if a.deps.Appearance == nil {
		a.deps.Appearance = &styles.Appearance{}
	}
	if a.deps.Theme.NoColor {
		a.status = "colors are off (--no-color / NO_COLOR), so there's no theme to switch"
		return nil
	}
	a.deps.Appearance.Toggle()
	mode := a.deps.Appearance.Mode
	if mode == styles.Strong {
		a.status = "theme: strong — screens painted in the club's colors (the pitch elsewhere)"
	} else {
		a.status = "theme: weak — your terminal's background, club colors as highlights"
	}
	if a.deps.SaveTheme != nil {
		if err := a.deps.SaveTheme(mode); err != nil {
			a.status += " (not saved: " + err.Error() + ")"
		} else {
			a.status += " (saved as default)"
		}
	}
	// Screens pushed later are built from a.deps, so they start in the
	// new mode too.
	a.deps.Theme = styles.BaseFor(mode, false)
	return a.broadcast(core.ThemeChangedMsg{Mode: mode, Theme: a.deps.Theme})
}

// openSearch returns to the search screen with its box focused: popping
// back to it when it is already on the stack, pushing one when it isn't
// (the app was opened straight onto a team, say).
func (a *App) openSearch() tea.Cmd {
	for i := len(a.stack) - 1; i >= 0; i-- {
		searcher, ok := a.stack[i].(core.Searcher)
		if !ok {
			continue
		}
		a.stack = a.stack[:i+1]
		a.ids = a.ids[:i+1]
		a.status = ""
		a.err = nil
		return searcher.FocusSearch()
	}
	if a.deps.Nav.Browse == nil {
		return nil
	}
	return core.Push(a.deps.Nav.Browse(a.deps, ""))
}

// capturingText reports whether the visible screen wants bare letters.
func (a *App) capturingText() bool {
	entry, ok := a.current().(core.TextEntry)
	return ok && entry.CapturingText()
}

// openCatalog pushes the league catalog, unless it is already showing.
func (a *App) openCatalog() tea.Cmd {
	if !a.catalogAvailable() {
		a.status = "the league catalog isn't available in this view — see  footui competitions"
		return nil
	}
	if a.current().Title() == catalog.Title {
		return nil
	}
	return core.Push(a.deps.Nav.Catalog(a.deps))
}

func (a *App) catalogAvailable() bool {
	return a.deps.Nav.Catalog != nil && len(a.deps.Catalog) > 0
}

// pop removes the top screen, quitting when the stack would be emptied.
func (a *App) pop() (tea.Model, tea.Cmd) {
	if len(a.stack) <= 1 {
		return a, tea.Quit
	}
	a.stack = a.stack[:len(a.stack)-1]
	a.ids = a.ids[:len(a.ids)-1]
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
	if a.splash != nil {
		return a.splashView()
	}
	if a.kick != nil {
		return a.kick.View()
	}
	return a.baseView()
}

// baseView is the screen with the app's chrome around it.
func (a *App) baseView() string {
	var top, bottom strings.Builder

	if crumb := a.breadcrumb(); crumb != "" {
		top.WriteString(crumb)
		top.WriteString("\n\n")
	}

	bottom.WriteString("\n\n")
	bottom.WriteString(a.footer())
	if line := a.statusLine(); line != "" {
		bottom.WriteString("\n")
		bottom.WriteString(line)
	}

	// A screen taller than what's left is cut from the bottom. Drawn
	// whole, the terminal would scroll its top away instead — the header
	// that says whose screen this is.
	room := a.height - strings.Count(top.String(), "\n") - strings.Count(bottom.String(), "\n") - 1
	body := a.current().View()
	if lines := strings.Split(body, "\n"); room > 0 && len(lines) > room {
		body = strings.Join(lines[:room], "\n")
	}

	return styles.Paint(top.String()+body+bottom.String(), a.theme(), a.width, a.height)
}

func (a *App) breadcrumb() string {
	if len(a.stack) <= 1 {
		return ""
	}
	titles := make([]string, 0, len(a.stack))
	for _, s := range a.stack {
		titles = append(titles, s.Title())
	}
	return a.theme().Breadcrumb.Render(strings.Join(titles, " › "))
}

// footer renders the current screen's key bindings. Screens declare
// bindings rather than help strings, so what the footer promises is always
// what the screen actually handles.
func (a *App) footer() string {
	// The global bindings are the app's, not the screen's, so they're
	// appended here rather than repeated in every screen's KeyMap.
	//
	// The expanded help lists every one. The one-line footer can't fit
	// them all beside a screen's own keys, so it keeps what gets a user
	// around and out, ordered so a narrow terminal cuts the least
	// essential first; the rest are one "?" away.
	var short, full []key.Binding
	refresh, canRefresh := a.refreshBinding()
	download := keys.Catalog
	canDownload := a.catalogAvailable() && a.current().Title() != catalog.Title
	_, isSearch := a.current().(core.Searcher)

	if a.capturingText() {
		// The text box takes "?" and every bare letter, so there is no
		// expanded help to defer to: the ctrl keys are all listed.
		download = keys.CatalogFromTextEntry
		short = []key.Binding{keys.Back, keys.QuitFromTextEntry}
		if canRefresh {
			short = append(short, refresh)
		}
		if canDownload {
			short = append(short, download)
		}
		short = append(short, keys.ThemeFromTextEntry)
		full = short
	} else {
		short = []key.Binding{keys.Back, keys.Help, keys.Quit, keys.Theme}
		if !isSearch {
			short = append(short, keys.Find)
		}
		if canRefresh {
			short = append(short, refresh)
			full = append(full, refresh)
		}
		if canDownload {
			full = append(full, download)
		}
		if !isSearch {
			full = append(full, keys.Find)
		}
		full = append(full, keys.Theme, keys.Help, keys.Back, keys.Quit, keys.Kickabout)
	}
	// The favourite key goes first in both views: it's the one action
	// that depends on what's on screen, so the user can't know it's there
	// without being told.
	if favorite, ok := a.favoriteBinding(); ok {
		short = append([]key.Binding{favorite}, short...)
		full = append([]key.Binding{favorite}, full...)
	}

	km := a.current().Keys()
	if slices.ContainsFunc(km.Short, func(b key.Binding) bool { return b.Help().Key == "esc" }) {
		// The screen has its own use for esc right now (clearing a
		// query), and advertising "esc back" alongside it would be wrong.
		isEsc := func(b key.Binding) bool { return b.Help().Key == "esc" }
		short = slices.DeleteFunc(slices.Clone(short), isEsc)
		full = slices.DeleteFunc(slices.Clone(full), isEsc)
	}
	km.Full = append(append([][]key.Binding{}, km.FullHelp()...), full)
	km.Short = append(append([]key.Binding{}, km.Short...), short...)

	a.help.Width = a.width
	a.help.Styles.ShortKey = a.theme().Keycap
	a.help.Styles.ShortDesc = a.theme().Help
	a.help.Styles.ShortSeparator = a.theme().Help
	a.help.Styles.FullKey = a.theme().Keycap
	a.help.Styles.FullDesc = a.theme().Help
	a.help.Styles.FullSeparator = a.theme().Help

	// help.Model's own width limit lets the last item through when the
	// ellipsis doesn't quite fit, so every line is clipped here as well.
	lines := strings.Split(a.help.View(km), "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, a.width, "…")
	}
	return strings.Join(lines, "\n")
}

// statusLine is the single row beneath the footer. Messages are cut to the
// terminal width: a wrapped status would push the chrome off the bottom of
// the screen.
func (a *App) statusLine() string {
	switch {
	case a.refreshing != "":
		return a.theme().Status.Render(components.Truncate(a.refreshing, a.width))
	case a.status != "":
		return a.theme().Status.Render(components.Truncate(a.status, a.width))
	case a.err != nil:
		return a.theme().Error.Render(components.Truncate("error: "+a.err.Error(), a.width))
	}
	return ""
}

// Run starts the program on the given screen and blocks until the user
// quits, behind a launch splash. startup is refresh work to queue
// immediately; waitForData keeps the splash up until the first of it
// lands, for a database with nothing to show yet.
func Run(deps core.Deps, initial core.Screen, waitForData bool, startup ...core.RefreshRequest) error {
	app := NewApp(deps, initial)
	app.ShowSplash(waitForData)
	app.QueueOnStart(startup...)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
