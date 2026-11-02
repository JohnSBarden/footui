// Package tui owns the navigation stack, global keys and chrome. It is the
// only package that imports the screens; they navigate via core.Navigator.
package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/trace"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/kickabout"
	"github.com/JohnSBarden/footui/internal/tui/logpanel"
	"github.com/JohnSBarden/footui/internal/tui/screens/browse"
	"github.com/JohnSBarden/footui/internal/tui/screens/catalog"
	"github.com/JohnSBarden/footui/internal/tui/screens/dashboard"
	"github.com/JohnSBarden/footui/internal/tui/screens/games"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
	"github.com/JohnSBarden/footui/internal/tui/screens/nation"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/ticker"
)

// App is the root Bubble Tea model.
type App struct {
	deps  core.Deps
	stack []core.Screen
	// zoneIDs are the clickable regions of the last frame drawn.
	zoneIDs []string
	// ids parallels stack, so a message from a screen beneath the top can
	// be routed back to it.
	ids    []int
	nextID int

	width  int
	height int

	status string
	err    error

	// refreshing replaces the status line while a refresh runs; unlike
	// status it survives keystrokes.
	refreshing string
	// active is nil when idle. batch collects outcomes until the queue
	// drains.
	active *core.RefreshRequest
	queue  []core.RefreshRequest
	batch  refreshBatch
	// stopping is set once the user asks to stop, until it reports back.
	stopActive context.CancelFunc
	stopping   bool
	// startup is refresh work to queue as soon as the app starts.
	startup []core.RefreshRequest

	help     help.Model
	showFull bool
	// showLog puts the log panel beside every screen; logTicking is set
	// while a repaint is pending.
	showLog    bool
	logTicking bool

	// splash is the launch screen, nil once it's gone.
	splash *splashState

	// newsBefore is the snapshot taken as the current batch started;
	// newsTurning is set while a tick is pending.
	news        ticker.Feed
	newsBefore  ticker.Snapshot
	newsTurning bool
	// clock stands in for time.Now in tests; nil is the real clock.
	clock func() time.Time

	// kick takes the keys while it runs; every other message still
	// reaches the stack, so the screen is current when the game ends.
	kick *kickabout.Model

	// framing is set while an animation frame is pending.
	framing bool
}

var _ tea.Model = (*App)(nil)

// NewApp constructs the root model with initial at the bottom of the stack.
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

// NewDeps builds the dependency bundle every screen is constructed with.
func NewDeps(ctx context.Context, repos storage.Repositories, theme styles.Theme) core.Deps {
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: theme, Favorites: core.NewFavorites(nil), Log: core.NewLog(nil)}
	deps.Nav = navigator()
	return deps
}

func navigator() core.Navigator {
	return core.Navigator{
		League: func(deps core.Deps, competitionID, seasonID string) core.Screen {
			return league.NewAt(deps, competitionID, seasonID)
		},
		Team: func(deps core.Deps, t models.Team, competitionID, seasonID string) core.Screen {
			// Every route to a national team lands on the nation screen.
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

// NewLeagueScreen constructs the League screen for a competition.
func NewLeagueScreen(deps core.Deps, competitionID string) core.Screen {
	return league.New(deps, competitionID)
}

// NewTeamScreen constructs the Team screen, or the nation screen for a
// national team.
func NewTeamScreen(deps core.Deps, t models.Team) core.Screen {
	if t.National {
		return nation.New(deps, t)
	}
	return team.New(deps, t, "", "")
}

// NewPlayerScreen constructs the Player screen.
func NewPlayerScreen(deps core.Deps, p models.Player) core.Screen {
	return player.New(deps, p, "")
}

// NewBrowseScreen constructs the Browse screen, optionally pre-filled with
// a search query.
func NewBrowseScreen(deps core.Deps, query string) core.Screen {
	return browse.New(deps, query)
}

// NewHomeScreen is the dashboard once anything is favourited, Browse until
// then.
func NewHomeScreen(deps core.Deps) core.Screen {
	if deps.Favorites.Len() > 0 {
		return dashboard.New(deps)
	}
	return browse.New(deps, "")
}

// QueueOnStart arranges for reqs to be refreshed as soon as the app
// starts.
func (a *App) QueueOnStart(reqs ...core.RefreshRequest) {
	a.startup = append(a.startup, reqs...)
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	init := a.current().Init()
	if a.splash != nil {
		init = tea.Batch(init, splashTimer(), splashFrame())
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

// frameEvery paces the animation of a screen that asks for it.
const frameEvery = 100 * time.Millisecond

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(core.FrameMsg); ok {
		a.framing = false
	}
	model, cmd := a.update(msg)
	return model, tea.Batch(cmd, a.nextFrame())
}

// nextFrame asks for an animation frame when the top screen is animating
// and none is pending; not while the splash or kickabout covers it.
func (a *App) nextFrame() tea.Cmd {
	if a.framing || a.splash != nil || a.kick != nil {
		return nil
	}
	if s, ok := a.current().(core.Animated); !ok || !s.Animating() {
		return nil
	}
	a.framing = true
	return tea.Tick(frameEvery, func(time.Time) tea.Msg { return core.FrameMsg{} })
}

func (a *App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if a.splash != nil {
		if mouse, ok := msg.(tea.MouseMsg); ok {
			if mouse.Action == tea.MouseActionPress {
				a.splash = nil
			}
			return a, nil
		}
		if key, ok := msg.(tea.KeyMsg); ok {
			// The key that skips the splash isn't passed on.
			a.splash = nil
			if key.String() == "ctrl+c" {
				return a, tea.Quit
			}
			return a, nil
		}
	}
	if _, ok := msg.(tea.MouseMsg); ok && a.kick != nil {
		// The kickabout is played on the keys.
		return a, nil
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

	case splashFrameMsg:
		return a, a.splashAnimate()

	case tea.WindowSizeMsg:
		// A terminal that can't report its size sends zeroes (some ptys,
		// CI); keep the default rather than collapse every column.
		if msg.Width <= 0 || msg.Height <= 0 {
			return a, nil
		}
		a.width, a.height = msg.Width, msg.Height
		// Resize the whole stack, so a popped-back screen isn't stale.
		var cmds []tea.Cmd
		for i, s := range a.stack {
			updated, cmd := s.Update(a.screenSize(i))
			a.stack[i] = updated
			cmds = append(cmds, a.routeFrom(i, cmd))
		}
		if a.kick != nil {
			// The pitch is built from the screen, so rebuild it.
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
		sized, sizeCmd := msg.Screen.Update(a.screenSize(len(a.stack) - 1))
		// A screen pushed mid-refresh learns what's in flight now rather
		// than at the next queue change.
		sized, queueCmd := sized.Update(core.RefreshQueueMsg{Active: a.active, Pending: slices.Clone(a.queue)})
		sized, logCmd := sized.Update(core.LogPanelMsg{Open: a.showLog})
		a.stack[len(a.stack)-1] = sized
		return a, tea.Batch(sizeCmd, queueCmd, logCmd, sized.Init())

	case core.PopMsg:
		return a.pop()

	case tea.MouseMsg:
		return a.handleMouse(msg)

	case routedMsg:
		return a, a.deliver(msg)

	case core.StatusMsg:
		a.status = msg.Text
		return a, nil

	case core.ErrMsg:
		// The requesting screen shows errors; the app keeps a copy so a
		// screen that ignores ErrMsg doesn't swallow it.
		a.err = msg.Err

	case tickerMsg:
		return a, a.turnNews()

	case logTickMsg:
		a.logTicking = false
		return a, a.logTick()

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

// handleGlobalKey reports whether it consumed key as a global binding.
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
	case "L":
		if a.capturingText() {
			return false, a, nil
		}
		return true, a, a.toggleLog()
	case "ctrl+k":
		return true, a, a.startKick()
	}
	return false, a, nil
}

// logTickMsg repaints the open panel in debug mode, whose lines arrive
// without a message of their own.
type logTickMsg struct{}

const logTickEvery = 500 * time.Millisecond

func (a *App) logTick() tea.Cmd {
	if !a.showLog || a.logTicking || !trace.On() {
		return nil
	}
	a.logTicking = true
	return tea.Tick(logTickEvery, func(time.Time) tea.Msg { return logTickMsg{} })
}

// toggleLog resizes the stack around the panel and tells each screen.
func (a *App) toggleLog() tea.Cmd {
	a.showLog = !a.showLog
	cmds := []tea.Cmd{a.logTick()}
	for i, s := range a.stack {
		updated, sizeCmd := s.Update(a.screenSize(i))
		updated, logCmd := updated.Update(core.LogPanelMsg{Open: a.showLog})
		a.stack[i] = updated
		cmds = append(cmds, a.routeFrom(i, sizeCmd), a.routeFrom(i, logCmd))
	}
	return tea.Batch(cmds...)
}

// logBeside is the panel's width while it shares the screen, zero when
// it's closed or takes the whole width.
func (a *App) logBeside() int {
	if !a.showLog {
		return 0
	}
	if w := logpanel.Width(a.width); w < a.width {
		return w
	}
	return 0
}

func (a *App) startKick() tea.Cmd {
	k := kickabout.New(a.baseView(), a.width, a.height, a.theme())
	a.kick = &k
	return k.Init()
}

// updateKick reports whether the game consumed msg; ctrl+c still quits.
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

// theme is the visible screen's own theme when it has one, else the app's.
func (a *App) theme() styles.Theme {
	if themed, ok := a.current().(core.ThemedScreen); ok {
		return themed.ScreenTheme()
	}
	return a.deps.Theme
}

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
	// Screens pushed later are built from a.deps.
	a.deps.Theme = styles.BaseFor(mode, false)
	return a.broadcast(core.ThemeChangedMsg{Mode: mode, Theme: a.deps.Theme})
}

// openSearch pops back to a search screen on the stack, or pushes one.
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

func (a *App) capturingText() bool {
	entry, ok := a.current().(core.TextEntry)
	return ok && entry.CapturingText()
}

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

// pop quits when the stack would be emptied.
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

// screenSize is the terminal minus chrome for stack index i. The first
// screen has no breadcrumb, so it gets those lines too.
func (a *App) screenSize(i int) tea.WindowSizeMsg {
	h := a.height - chromeHeight
	if i > 0 {
		h -= crumbHeight
	}
	w := a.width
	if p := a.logBeside(); p > 0 {
		w -= p + logpanel.Gap
	}
	return tea.WindowSizeMsg{Width: w, Height: h}
}

// chromeHeight: gap, footer, status line. crumbHeight: breadcrumb and gap.
const (
	chromeHeight = 3
	crumbHeight  = 2
)

// View implements tea.Model.
func (a *App) View() string {
	var v string
	switch {
	case a.splash != nil:
		v = a.splashView()
	case a.kick != nil:
		v = a.kick.View()
	default:
		v = a.baseView()
	}
	a.zoneIDs = components.MarkedZones()
	return components.ScanZones(v)
}

// handleMouse maps the wheel to arrow keys and a click to a ClickMsg for
// the zone it landed in.
func (a *App) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp && msg.Action == tea.MouseActionPress:
		return a.Update(tea.KeyMsg{Type: tea.KeyUp})
	case msg.Button == tea.MouseButtonWheelDown && msg.Action == tea.MouseActionPress:
		return a.Update(tea.KeyMsg{Type: tea.KeyDown})
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		id, ok := components.ZoneAt(msg, a.zoneIDs)
		if !ok {
			return a, nil
		}
		return a, a.click(id)
	}
	return a, nil
}

// click sends a click to the screen, except the log button's.
func (a *App) click(id string) tea.Cmd {
	a.status = ""
	if id == "log" {
		return a.toggleLog()
	}
	updated, cmd := a.current().Update(core.ClickMsg{ID: id})
	a.stack[len(a.stack)-1] = updated
	return cmd
}

func (a *App) baseView() string {
	var top, bottom strings.Builder

	if crumb := a.breadcrumb(); crumb != "" {
		top.WriteString(crumb)
		top.WriteString("\n\n")
	}

	// The status line keeps its row when empty, so the footer doesn't jump.
	bottom.WriteString("\n\n")
	bottom.WriteString(a.footer())
	bottom.WriteString("\n")
	bottom.WriteString(a.statusLine())

	// Cut a tall screen from the bottom (else the terminal scrolls its
	// header away); pad a short one so the footer stays at the bottom.
	room := a.height - strings.Count(top.String(), "\n") - strings.Count(bottom.String(), "\n")
	body := a.current().View()
	if room > 0 {
		lines := strings.Split(body, "\n")
		if len(lines) > room {
			lines = lines[:room]
		}
		for len(lines) < room {
			lines = append(lines, "")
		}
		body = strings.Join(lines, "\n")
		if a.showLog {
			body = a.withLog(body, room)
		}
	}

	return styles.Paint(top.String()+body+bottom.String(), a.theme(), a.width, a.height)
}

// withLog puts the panel beside body, or in its place on a narrow
// terminal.
func (a *App) withLog(body string, height int) string {
	p := logpanel.Panel{
		Theme: a.theme(),
		Log:   a.deps.Log,
		Queue: core.RefreshQueueMsg{Active: a.active, Pending: a.queue},
		Pause: a.deps.ProviderPause,
		Now:   a.now(),
	}
	w := a.logBeside()
	if w == 0 {
		return p.View(a.width, height)
	}
	return logpanel.Beside(body, p.View(w, height), a.width-w-logpanel.Gap)
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

// footer renders the current screen's key bindings plus the global ones.
func (a *App) footer() string {
	// The short footer is ordered so a narrow terminal cuts the least
	// essential first; the rest are one "?" away.
	var short, full []key.Binding
	refresh, canRefresh := a.refreshBinding()
	download := keys.Catalog
	canDownload := a.catalogAvailable() && a.current().Title() != catalog.Title
	_, isSearch := a.current().(core.Searcher)

	if a.capturingText() {
		// The text box takes "?", so list all the ctrl keys here.
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
		full = append(full, keys.Theme, keys.Log, keys.Help, keys.Back, keys.Quit, keys.Kickabout)
	}
	// Favourite goes first: it's the one action that depends on what's on
	// screen.
	if favorite, ok := a.favoriteBinding(); ok {
		short = append([]key.Binding{favorite}, short...)
		full = append([]key.Binding{favorite}, full...)
	}

	km := a.current().Keys()
	if slices.ContainsFunc(km.Short, func(b key.Binding) bool { return b.Help().Key == "esc" }) {
		// The screen has its own use for esc right now; drop "esc back".
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

	// help.Model's width limit lets the last item overflow when the
	// ellipsis doesn't fit, so clip here too.
	lines := strings.Split(a.help.View(km), "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, a.width, "…")
	}
	return strings.Join(lines, "\n")
}

// statusLine is cut to the width: a wrapped status would push the chrome
// off the screen.
func (a *App) statusLine() string {
	switch {
	case a.refreshing != "":
		return a.theme().Status.Render(components.Truncate(a.refreshing, a.width))
	case a.status != "":
		return a.theme().Status.Render(components.Truncate(a.status, a.width))
	case a.err != nil:
		return a.theme().Error.Render(components.Truncate("error: "+a.err.Error(), a.width))
	case a.news.Len() > 0:
		return a.theme().Status.Render(components.Truncate(a.news.Current(), a.width))
	}
	return ""
}

// Run starts the program behind a splash and blocks until the user quits.
// waitForData keeps the splash up until the first startup refresh lands.
func Run(deps core.Deps, initial core.Screen, waitForData bool, startup ...core.RefreshRequest) error {
	app := NewApp(deps, initial)
	app.ShowSplash(waitForData)
	app.QueueOnStart(startup...)
	components.EnableZones()
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
