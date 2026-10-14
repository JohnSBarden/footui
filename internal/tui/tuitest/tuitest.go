// Package tuitest holds helpers for driving screens in tests without a
// terminal: Bubble Tea's Update/View are ordinary functions, so a screen
// can be stepped through its load sequence and its rendered output
// compared directly.
package tuitest

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// maxSteps bounds Settle so a screen that keeps issuing commands (a bug)
// fails the test instead of hanging it.
const maxSteps = 20

func init() {
	// Golden output must not depend on whether the test runner's stdout
	// happens to be a terminal, so color is pinned off for every test in
	// every package that imports this one.
	lipgloss.SetColorProfile(termenv.Ascii)
}

// Settle runs a screen's Init command and feeds the resulting messages back
// in until no commands remain, leaving the screen in its loaded state.
func Settle(t *testing.T, s core.Screen) core.Screen {
	t.Helper()
	return Drive(t, s, s.Init())
}

// Drive feeds cmd's message back into s, repeating until the screen stops
// issuing commands. tea.Batch results are expanded and each command run,
// mirroring what the real Bubble Tea runtime does — a screen whose Init
// batches a blink with a data load must still end up loaded.
func Drive(t *testing.T, s core.Screen, cmd tea.Cmd) core.Screen {
	t.Helper()

	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps >= maxSteps {
			t.Fatalf("tuitest: screen still issuing commands after %d steps", maxSteps)
		}

		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}

		if isTimer(next) {
			// A timer (the text cursor's blink) isn't a data load.
			// Waiting on it would add half a second per test and tell
			// us nothing about the screen's behaviour.
			continue
		}
		msg, ok := runCmd(next)
		if !ok {
			t.Fatalf("tuitest: a command gave no message within %s", cmdTimeout)
		}
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}

		updated, cmd := s.Update(msg)
		s = updated
		queue = append(queue, cmd)
	}
	return s
}

// cmdTimeout is how long a command gets to produce its message. Queries
// against an in-memory database return far inside it, even under the race
// detector with every package's tests running at once — which is why
// timers are recognised by what they are (isTimer) rather than by being
// slow: a short timeout made loads that took a little longer under load
// look like timers, and tests flaked.
const cmdTimeout = 10 * time.Second

// timerFuncs are the functions behind the commands that wait on a clock
// rather than load anything.
var timerFuncs = []string{
	"github.com/charmbracelet/bubbles/cursor.",
	"github.com/charmbracelet/bubbletea.Tick",
	"github.com/charmbracelet/bubbletea.Every",
}

// isTimer reports whether cmd is a timer: a blinking cursor or a tick.
func isTimer(cmd tea.Cmd) bool {
	fn := runtime.FuncForPC(reflect.ValueOf(cmd).Pointer())
	if fn == nil {
		return false
	}
	for _, prefix := range timerFuncs {
		if strings.HasPrefix(fn.Name(), prefix) {
			return true
		}
	}
	return false
}

// runCmd runs a command, reporting false if it didn't return in time.
func runCmd(cmd tea.Cmd) (tea.Msg, bool) {
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		return msg, true
	case <-time.After(cmdTimeout):
		return nil, false
	}
}

// Key sends a keystroke to a screen and settles any command it returns.
// The name is a Bubble Tea key string ("down", "enter", "left").
func Key(t *testing.T, s core.Screen, name string) core.Screen {
	t.Helper()
	updated, cmd := s.Update(keyMsg(name))
	return Drive(t, updated, cmd)
}

// Resize sends a window-size message, since most screens lay out against
// the terminal width.
func Resize(t *testing.T, s core.Screen, width, height int) core.Screen {
	t.Helper()
	updated, cmd := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return Drive(t, updated, cmd)
}

// CapturedCmd runs a command and returns the message it produced without
// feeding it to a screen, for asserting on navigation messages.
func CapturedCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// keyMsg builds the tea.KeyMsg for a key name. Named keys map to their
// tea.KeyType; anything else is treated as literal runes.
func keyMsg(name string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab,
		"shift+tab": tea.KeyShiftTab, "home": tea.KeyHome, "end": tea.KeyEnd,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "backspace": tea.KeyBackspace,
		"space": tea.KeySpace,
	}
	if kt, ok := named[name]; ok {
		return tea.KeyMsg{Type: kt}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}
