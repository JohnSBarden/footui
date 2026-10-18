package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// fakeScreen records what the app sends it, so navigation and key routing
// can be asserted without a real screen's data loading.
type fakeScreen struct {
	name        string
	initialized *bool
	keys        *[]string
	width       *int
	capturing   bool
	keyMap      core.KeyMap
	// target is what RefreshTarget reports; refreshed records the
	// RefreshedMsgs the screen was handed.
	target    string
	refreshed *[]string
}

func newFake(name string) fakeScreen {
	return fakeScreen{
		name:        name,
		initialized: new(bool),
		keys:        new([]string),
		width:       new(int),
		refreshed:   new([]string),
	}
}

func (f fakeScreen) Init() tea.Cmd {
	*f.initialized = true
	return nil
}

func (f fakeScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		*f.keys = append(*f.keys, msg.String())
	case tea.WindowSizeMsg:
		*f.width = msg.Width
	case core.RefreshedMsg:
		*f.refreshed = append(*f.refreshed, msg.CompetitionID)
	}
	return f, nil
}

func (f fakeScreen) RefreshTarget() (string, bool) { return f.target, f.target != "" }

func (f fakeScreen) View() string        { return "view:" + f.name }
func (f fakeScreen) Title() string       { return f.name }
func (f fakeScreen) CapturingText() bool { return f.capturing }

func newApp(initial core.Screen) *App {
	return NewApp(core.Deps{Theme: styles.Plain()}, initial)
}

// isQuit reports whether a command produced Bubble Tea's quit message.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func keyPress(name string) tea.KeyMsg {
	switch name {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "ctrl+r":
		return tea.KeyMsg{Type: tea.KeyCtrlR}
	case "ctrl+g":
		return tea.KeyMsg{Type: tea.KeyCtrlG}
	case "ctrl+t":
		return tea.KeyMsg{Type: tea.KeyCtrlT}
	case "ctrl+f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

func TestPushAddsAndInitializesAScreen(t *testing.T) {
	root := newFake("Browse")
	pushed := newFake("League")
	app := newApp(root)

	updated, _ := app.Update(core.PushMsg{Screen: pushed})
	app = updated.(*App)

	if !*pushed.initialized {
		t.Error("a pushed screen must be initialized, or it never loads its data")
	}
	if got := app.View(); !strings.Contains(got, "view:League") {
		t.Errorf("expected the pushed screen to be visible, got %q", got)
	}
}

func TestEscPopsBackToThePreviousScreen(t *testing.T) {
	app := newApp(newFake("Browse"))
	updated, _ := app.Update(core.PushMsg{Screen: newFake("League")})
	app = updated.(*App)

	updated, cmd := app.Update(keyPress("esc"))
	app = updated.(*App)

	if isQuit(cmd) {
		t.Fatal("esc with a screen underneath should pop, not quit")
	}
	if got := app.View(); !strings.Contains(got, "view:Browse") {
		t.Errorf("expected to be back on Browse, got %q", got)
	}
}

func TestEscOnTheLastScreenQuits(t *testing.T) {
	app := newApp(newFake("Browse"))

	_, cmd := app.Update(keyPress("esc"))
	if !isQuit(cmd) {
		t.Error("esc with nothing to pop back to should quit")
	}
}

func TestQQuitsFromAnOrdinaryScreen(t *testing.T) {
	app := newApp(newFake("League"))

	_, cmd := app.Update(keyPress("q"))
	if !isQuit(cmd) {
		t.Error("q should quit")
	}
}

// Browse's search box needs the literal character, or a club with a "q" in
// its name would be unsearchable.
func TestQReachesAScreenThatIsCapturingText(t *testing.T) {
	screen := newFake("Browse")
	screen.capturing = true
	app := newApp(screen)

	_, cmd := app.Update(keyPress("q"))
	if isQuit(cmd) {
		t.Fatal("q must not quit while a screen is capturing text")
	}
	if len(*screen.keys) != 1 || (*screen.keys)[0] != "q" {
		t.Errorf("screen received %v, want the literal q", *screen.keys)
	}
}

// ctrl+c is the one binding that must always work, text entry or not.
func TestCtrlCAlwaysQuits(t *testing.T) {
	screen := newFake("Browse")
	screen.capturing = true
	app := newApp(screen)

	_, cmd := app.Update(keyPress("ctrl+c"))
	if !isQuit(cmd) {
		t.Error("ctrl+c must quit even while a screen captures text")
	}
}

// A screen popped back to must be laid out for the current terminal, not
// the size it last saw.
func TestResizeReachesEveryScreenInTheStack(t *testing.T) {
	root := newFake("Browse")
	pushed := newFake("League")

	app := newApp(root)
	updated, _ := app.Update(core.PushMsg{Screen: pushed})
	app = updated.(*App)

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if *root.width != 120 {
		t.Errorf("background screen width = %d, want 120", *root.width)
	}
	if *pushed.width != 120 {
		t.Errorf("foreground screen width = %d, want 120", *pushed.width)
	}
}

func TestBreadcrumbShowsThePathOnceNested(t *testing.T) {
	app := newApp(newFake("Browse"))
	if strings.Contains(app.View(), "›") {
		t.Error("a single screen needs no breadcrumb")
	}

	updated, _ := app.Update(core.PushMsg{Screen: newFake("Arsenal")})
	app = updated.(*App)

	if got := app.View(); !strings.Contains(got, "Browse › Arsenal") {
		t.Errorf("expected a breadcrumb, got %q", got)
	}
}

func TestStatusMessageIsShownThenClearedOnNextKeypress(t *testing.T) {
	app := newApp(newFake("League"))

	updated, _ := app.Update(core.StatusMsg{Text: "nothing to open"})
	app = updated.(*App)
	if !strings.Contains(app.View(), "nothing to open") {
		t.Error("status message not shown")
	}

	updated, _ = app.Update(keyPress("j"))
	app = updated.(*App)
	if strings.Contains(app.View(), "nothing to open") {
		t.Error("a stale status message should clear once the user acts")
	}
}

// A screen that ignores ErrMsg must not swallow the failure silently.
func TestErrorIsSurfacedInChrome(t *testing.T) {
	app := newApp(newFake("League"))

	updated, _ := app.Update(core.ErrMsg{Err: errFake{}})
	app = updated.(*App)

	if got := app.View(); !strings.Contains(got, "boom") {
		t.Errorf("expected the error in the status line, got %q", got)
	}
}

func TestPushOfANilScreenIsIgnored(t *testing.T) {
	app := newApp(newFake("League"))

	updated, _ := app.Update(core.PushMsg{Screen: nil})
	app = updated.(*App)

	if got := app.View(); !strings.Contains(got, "view:League") {
		t.Errorf("a nil push should leave the stack alone, got %q", got)
	}
}

type errFake struct{}

func (errFake) Error() string { return "boom" }

func (f fakeScreen) Keys() core.KeyMap { return f.keyMap }

// Screens declare bindings, not help text, and the app renders the footer
// from them — so what the footer promises is always what the screen
// actually handles.
func TestFooterIsRenderedFromTheScreensBindings(t *testing.T) {
	screen := newFake("League")
	screen.keyMap = core.KeyMap{Short: []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open club")),
	}}
	app := newApp(screen)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	out := app.View()
	if !strings.Contains(out, "open club") {
		t.Errorf("footer should show the screen's own binding\n---\n%s", out)
	}
	// The global bindings belong to the app, not to each screen.
	for _, want := range []string{"quit", "back", "keys"} {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing the global %q binding\n---\n%s", want, out)
		}
	}
}

func TestQuestionMarkTogglesExpandedHelp(t *testing.T) {
	screen := newFake("Team")
	screen.keyMap = core.KeyMap{
		Short: []key.Binding{key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))},
		Full: [][]key.Binding{
			{key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down"))},
		},
	}
	app := newApp(screen)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if strings.Contains(app.View(), "page down") {
		t.Fatal("the expanded-only binding should be hidden by default")
	}

	updated, _ := app.Update(keyPress("?"))
	app = updated.(*App)
	if !strings.Contains(app.View(), "page down") {
		t.Error("? should expand the help")
	}

	updated, _ = app.Update(keyPress("?"))
	app = updated.(*App)
	if strings.Contains(app.View(), "page down") {
		t.Error("? should collapse the help again")
	}
}

// Expanded help must still say how to leave it.
func TestExpandedHelpIncludesTheGlobalBindings(t *testing.T) {
	screen := newFake("Team")
	screen.keyMap = core.KeyMap{
		Short: []key.Binding{key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))},
		Full:  [][]key.Binding{{key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "first"))}},
	}
	app := newApp(screen)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ := app.Update(keyPress("?"))

	if out := updated.(*App).View(); !strings.Contains(out, "quit") {
		t.Errorf("expanded help must still show how to get out\n---\n%s", out)
	}
}

// A search box must receive a literal "?" rather than having the app
// swallow it as a help toggle.
func TestQuestionMarkReachesAScreenCapturingText(t *testing.T) {
	screen := newFake("Browse")
	screen.capturing = true
	app := newApp(screen)

	app.Update(keyPress("?"))
	if len(*screen.keys) != 1 || (*screen.keys)[0] != "?" {
		t.Errorf("screen received %v, want the literal ?", *screen.keys)
	}
}

// A screen with a focused text box swallows "q" and "?", so the footer
// must not advertise them — a user would discover the lie by typing a
// search and finding nothing happens.
func TestFooterOffersCtrlCWhileAScreenCapturesText(t *testing.T) {
	screen := newFake("Browse")
	screen.capturing = true
	app := newApp(screen)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	out := app.View()
	if !strings.Contains(out, "ctrl+c") {
		t.Errorf("footer should offer ctrl+c to quit a text-entry screen\n---\n%s", out)
	}
	if strings.Contains(out, "q quit") {
		t.Errorf("footer must not advertise q as quit while a text box has it\n---\n%s", out)
	}
	if strings.Contains(out, "? keys") {
		t.Errorf("footer must not advertise ? while a text box has it\n---\n%s", out)
	}
}

func TestFooterOffersQOnOrdinaryScreens(t *testing.T) {
	app := newApp(newFake("League"))
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if out := app.View(); !strings.Contains(out, "q") || !strings.Contains(out, "quit") {
		t.Errorf("an ordinary screen should offer q to quit\n---\n%s", out)
	}
}

// A pty with no winsize set, and some CI runners, report 0x0. Taking that
// literally collapses every table column to its minimum and truncates club
// names to nothing, so it must be ignored rather than believed.
func TestZeroSizedTerminalReportIsIgnored(t *testing.T) {
	screen := newFake("League")
	app := newApp(screen)

	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if *screen.width != 120 {
		t.Fatalf("setup: width = %d", *screen.width)
	}

	app.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	if *screen.width != 120 {
		t.Errorf("a 0x0 report overwrote a known width with %d", *screen.width)
	}
}

func TestUnsizedTerminalKeepsAUsableDefault(t *testing.T) {
	screen := newFake("League")
	app := newApp(screen)

	// The very first message, before any real size is known.
	app.Update(tea.WindowSizeMsg{Width: 0, Height: 0})

	if a := app.width; a < 80 {
		t.Errorf("app width fell to %d; an unsized terminal should keep a usable default", a)
	}
}

// fakeRefresher records the competition it was asked for and plays back a
// scripted refresh: each progress line, then the summary or error.
type fakeRefresher struct {
	called   []string
	requests []core.RefreshRequest
	progress []string
	// errFor fails only the named competitions, when set.
	errFor  map[string]error
	summary string
	err     error
}

func (f *fakeRefresher) fn() core.Refresher {
	return func(ctx context.Context, req core.RefreshRequest, progress func(string)) (string, error) {
		f.called = append(f.called, req.CompetitionID)
		f.requests = append(f.requests, req)
		for _, p := range f.progress {
			progress(p)
		}
		if err, ok := f.errFor[req.CompetitionID]; ok {
			return "", err
		}
		return f.summary, f.err
	}
}

// drive runs a command and feeds its message back into the app, repeating
// until the chain of commands ends — the synchronous equivalent of the
// Bubble Tea runtime, minus the goroutines. Views seen along the way are
// returned so a test can assert on what the user saw mid-flight.
func drive(t *testing.T, app *App, cmd tea.Cmd) (*App, []string) {
	t.Helper()
	var views []string
	queue := []tea.Cmd{cmd}
	for i := 0; len(queue) > 0; i++ {
		if i > 200 {
			t.Fatal("command chain did not terminate")
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if msg == nil {
			continue
		}
		updated, follow := app.Update(msg)
		app = updated.(*App)
		views = append(views, app.View())
		queue = append(queue, follow)
	}
	return app, views
}

func newAppWithRefresh(initial core.Screen, r *fakeRefresher) *App {
	return NewApp(core.Deps{Theme: styles.Plain(), Refresh: r.fn()}, initial)
}

func TestRRefreshesTheCurrentScreensCompetition(t *testing.T) {
	root := newFake("Browse")
	root.target = "eng-premier-league"
	league := newFake("League")
	league.target = "esp-la-liga"
	r := &fakeRefresher{progress: []string{"attempt 1 of 3 timed out"}, summary: "20 teams, 500 players"}
	app := newAppWithRefresh(root, r)
	updated, _ := app.Update(core.PushMsg{Screen: league})
	app = updated.(*App)

	updated, cmd := app.Update(keyPress("r"))
	app = updated.(*App)
	if cmd == nil {
		t.Fatal("r should start a refresh")
	}
	if got := app.View(); !strings.Contains(got, "🔄 Refreshing esp-la-liga") {
		t.Errorf("the user should be told a refresh started, got %q", got)
	}

	app, views := drive(t, app, cmd)

	if len(r.called) != 1 || r.called[0] != "esp-la-liga" {
		t.Errorf("refreshed %v, want the visible screen's competition", r.called)
	}
	if !strings.Contains(strings.Join(views, "\n"), "attempt 1 of 3 timed out") {
		t.Error("progress lines should be shown while the refresh runs")
	}
	if got := app.View(); !strings.Contains(got, "20 teams, 500 players") {
		t.Errorf("the summary should be shown when the refresh completes, got %q", got)
	}
	// Every screen on the stack hears about it, not just the visible one.
	for _, s := range []fakeScreen{root, league} {
		if got := *s.refreshed; len(got) != 1 || got[0] != "esp-la-liga" {
			t.Errorf("%s received RefreshedMsg %v, want one for esp-la-liga", s.name, got)
		}
	}
}

func TestRefreshFailureIsShownAndNothingReloads(t *testing.T) {
	screen := newFake("League")
	screen.target = "eng-premier-league"
	r := &fakeRefresher{err: errors.New("no response from understat.com within 15s")}
	app := newAppWithRefresh(screen, r)

	updated, cmd := app.Update(keyPress("r"))
	app, _ = drive(t, updated.(*App), cmd)

	if got := app.View(); !strings.Contains(got, "no response from understat.com") {
		t.Errorf("the failure should be shown, got %q", got)
	}
	if len(*screen.refreshed) != 0 {
		t.Error("a failed refresh must not tell screens to reload")
	}
}

func TestRIsIgnoredWhileARefreshIsRunning(t *testing.T) {
	screen := newFake("League")
	screen.target = "eng-premier-league"
	r := &fakeRefresher{}
	app := newAppWithRefresh(screen, r)

	updated, first := app.Update(keyPress("r"))
	app = updated.(*App)
	updated, second := app.Update(keyPress("r"))
	app = updated.(*App)

	drive(t, app, first)
	drive(t, app, second)
	if len(r.called) != 1 {
		t.Errorf("refresh ran %d times for two presses mid-flight, want 1", len(r.called))
	}
}

func TestRExplainsWhenTheScreenHasNothingToRefresh(t *testing.T) {
	r := &fakeRefresher{}
	app := newAppWithRefresh(newFake("Player"), r)

	updated, cmd := app.Update(keyPress("r"))
	app, _ = drive(t, updated.(*App), cmd)

	if len(r.called) != 0 {
		t.Errorf("refreshed %v from a screen with no target", r.called)
	}
	if got := app.View(); !strings.Contains(got, "open a league") {
		t.Errorf("the user should be told where refresh works, got %q", got)
	}
}

func TestRExplainsWhenNoRefresherIsWired(t *testing.T) {
	screen := newFake("League")
	screen.target = "eng-premier-league"
	app := newApp(screen)

	updated, cmd := app.Update(keyPress("r"))
	app, _ = drive(t, updated.(*App), cmd)

	if got := app.View(); !strings.Contains(got, "footui refresh --competition eng-premier-league") {
		t.Errorf("without a refresher the CLI command should be offered, got %q", got)
	}
}

// Browse's search box needs the literal "r"; ctrl+r is the refresh key
// there, and the footer must say so.
func TestCtrlRRefreshesWhileAScreenCapturesText(t *testing.T) {
	screen := newFake("Browse")
	screen.capturing = true
	screen.target = "eng-premier-league"
	r := &fakeRefresher{}
	app := newAppWithRefresh(screen, r)

	updated, cmd := app.Update(keyPress("r"))
	app = updated.(*App)
	drive(t, app, cmd)
	if len(r.called) != 0 {
		t.Fatal("r must reach the text box, not refresh")
	}
	if len(*screen.keys) != 1 || (*screen.keys)[0] != "r" {
		t.Errorf("screen received %v, want the literal r", *screen.keys)
	}

	updated, cmd = app.Update(keyPress("ctrl+r"))
	app = updated.(*App)
	drive(t, app, cmd)
	if len(r.called) != 1 {
		t.Error("ctrl+r should refresh while text is being captured")
	}
	if footer := app.footer(); !strings.Contains(footer, "ctrl+r") || !strings.Contains(footer, "refresh") {
		t.Errorf("footer should offer ctrl+r to refresh, got %q", footer)
	}
}

func TestFooterOffersRefreshOnlyWhereItWorks(t *testing.T) {
	league := newFake("League")
	league.target = "eng-premier-league"
	if footer := newAppWithRefresh(league, &fakeRefresher{}).footer(); !strings.Contains(footer, "refresh") {
		t.Errorf("a screen with a refresh target should advertise it, got %q", footer)
	}
	if footer := newAppWithRefresh(newFake("Player"), &fakeRefresher{}).footer(); strings.Contains(footer, "refresh") {
		t.Errorf("a screen with nothing to refresh should not advertise it, got %q", footer)
	}
}

// The status line has one row; a long message must be cut to fit rather
// than wrapping and pushing the chrome off the bottom of the screen.
func TestStatusLineIsTruncatedToTheTerminalWidth(t *testing.T) {
	app := newApp(newFake("League"))
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	app = updated.(*App)

	long := strings.Repeat("x", 80)
	updated, _ = app.Update(core.ErrMsg{Err: errors.New(long)})
	app = updated.(*App)

	for _, line := range strings.Split(app.View(), "\n") {
		if len([]rune(line)) > 40 {
			t.Errorf("line wider than the 40-column terminal: %q", line)
		}
	}
	if !strings.Contains(app.View(), "…") {
		t.Error("a truncated status line should end in an ellipsis")
	}
}
