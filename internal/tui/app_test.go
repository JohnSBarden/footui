package tui

import (
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
}

func newFake(name string) fakeScreen {
	return fakeScreen{
		name:        name,
		initialized: new(bool),
		keys:        new([]string),
		width:       new(int),
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
	}
	return f, nil
}

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
