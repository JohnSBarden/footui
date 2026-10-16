package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// searchFake is a fake search screen: it counts focus requests and
// consumes esc while it has something to unwind.
type searchFake struct {
	fakeScreen
	focused  *int
	unwinded *bool
}

func newSearchFake() searchFake {
	return searchFake{fakeScreen: newFake("Browse"), focused: new(int), unwinded: new(bool)}
}

func (s searchFake) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	s.fakeScreen.Update(msg)
	return s, nil
}

func (s searchFake) FocusSearch() tea.Cmd {
	*s.focused++
	return nil
}

func (s searchFake) Escape() (bool, tea.Cmd) {
	if *s.unwinded {
		return false, nil
	}
	*s.unwinded = true
	return true, nil
}

func TestSlashReturnsToTheSearchScreenOnTheStack(t *testing.T) {
	search := newSearchFake()
	app := newApp(search)
	app.Update(core.PushMsg{Screen: newFake("League")})
	app.Update(core.PushMsg{Screen: newFake("Team")})

	app.Update(keyPress("/"))

	if len(app.stack) != 1 {
		t.Fatalf("stack has %d screens; / should pop back to search", len(app.stack))
	}
	if *search.focused != 1 {
		t.Error("/ should focus the search box")
	}
}

func TestSlashPushesASearchScreenWhenNoneIsOnTheStack(t *testing.T) {
	deps := core.Deps{Theme: styles.Plain()}
	deps.Nav.Browse = func(core.Deps, string) core.Screen { return newSearchFake() }
	app := NewApp(deps, newFake("Team"))

	_, cmd := app.Update(keyPress("/"))

	if _, ok := cmd().(core.PushMsg); !ok {
		t.Error("/ with no search screen open should push one")
	}
}

func TestSlashIsLiteralWhileTyping(t *testing.T) {
	typing := newFake("Browse")
	typing.capturing = true
	app := newApp(typing)

	app.Update(keyPress("/"))

	if got := *typing.keys; len(got) != 1 || got[0] != "/" {
		t.Errorf("a screen capturing text should receive /, got %v", got)
	}
}

func TestEscLetsTheScreenUnwindBeforeGoingBack(t *testing.T) {
	app := newApp(newFake("League"))
	search := newSearchFake()
	app.Update(core.PushMsg{Screen: search})

	app.Update(keyPress("esc"))
	if len(app.stack) != 2 {
		t.Fatal("the first esc belongs to the screen, which had a query to clear")
	}
	app.Update(keyPress("esc"))
	if len(app.stack) != 1 {
		t.Error("once the screen has nothing to unwind, esc goes back")
	}
}

func TestStoppingDownloadsCancelsTheRunningOneAndEmptiesTheQueue(t *testing.T) {
	r := &fakeRefresher{}
	app := newAppWithRefresh(newFake("Catalog"), r)
	app.Update(core.RefreshRequestMsg{Requests: []core.RefreshRequest{
		{CompetitionID: "eng-premier-league"}, {CompetitionID: "esp-la-liga"},
	}})
	if app.active == nil || len(app.queue) != 1 {
		t.Fatal("expected one refresh running and one queued")
	}

	app.Update(core.CancelRefreshMsg{})
	if len(app.queue) != 0 {
		t.Error("stopping should drop what's queued")
	}

	// The running refresh reports back with the cancellation error.
	app.Update(refreshMsg{event: refreshEvent{done: true, err: context.Canceled}})
	if app.active != nil {
		t.Error("the app should be idle once the stopped refresh reports back")
	}
	if app.err != nil {
		t.Errorf("a stop the user asked for is not an error, got %v", app.err)
	}
	if !strings.Contains(app.View(), "Downloads stopped") {
		t.Errorf("the status line should confirm the stop\n%s", app.View())
	}
}

func TestStoppingWithNothingRunningSaysSo(t *testing.T) {
	app := newAppWithRefresh(newFake("Catalog"), &fakeRefresher{})
	app.Update(core.CancelRefreshMsg{})
	if !strings.Contains(app.View(), "nothing is downloading") {
		t.Errorf("expected an explanation\n%s", app.View())
	}
}

func TestThemeToggleSavesTheChoice(t *testing.T) {
	var saved []styles.Mode
	deps := core.Deps{Theme: styles.BaseFor(styles.Weak, false), Appearance: &styles.Appearance{}}
	deps.SaveTheme = func(m styles.Mode) error { saved = append(saved, m); return nil }
	app := NewApp(deps, newFake("League"))

	app.Update(keyPress("t"))

	if len(saved) != 1 || saved[0] != styles.Strong {
		t.Errorf("saved %v, want [strong]", saved)
	}
	if !strings.Contains(app.View(), "saved as default") {
		t.Errorf("the status line should say the choice was saved\n%s", app.View())
	}
}

func TestThemeToggleExplainsWhyItWasNotSaved(t *testing.T) {
	deps := core.Deps{Theme: styles.BaseFor(styles.Weak, false), Appearance: &styles.Appearance{}}
	deps.SaveTheme = func(styles.Mode) error { return errors.New("FOOTUI_THEME overrides the settings file") }
	app := NewApp(deps, newFake("League"))
	app.Update(tea.WindowSizeMsg{Width: 200, Height: 30})

	app.Update(keyPress("t"))

	if !strings.Contains(app.View(), "not saved: FOOTUI_THEME") {
		t.Errorf("expected the reason it wasn't saved\n%s", app.View())
	}
}
