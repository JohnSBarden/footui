// Package tuitest drives screens in tests without a terminal.
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
	// Pin colour off so golden output doesn't depend on stdout being a
	// terminal; applies to every importing package.
	lipgloss.SetColorProfile(termenv.Ascii)
}

// Settle runs s's Init and drives it until no commands remain.
func Settle(t *testing.T, s core.Screen) core.Screen {
	t.Helper()
	return Drive(t, s, s.Init())
}

// Drive feeds cmd's message back into s until it stops issuing commands,
// expanding tea.Batch as the runtime does. Timers are skipped.
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
			// A timer (cursor blink) isn't a data load; skip it.
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

// cmdTimeout is generous on purpose: timers are recognised by isTimer, not
// by being slow, since a short timeout flaked under -race.
const cmdTimeout = 10 * time.Second

var timerFuncs = []string{
	"github.com/charmbracelet/bubbles/cursor.",
	"github.com/charmbracelet/bubbletea.Tick",
	"github.com/charmbracelet/bubbletea.Every",
}

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

// Key sends a Bubble Tea key string ("down", "enter") and settles.
func Key(t *testing.T, s core.Screen, name string) core.Screen {
	t.Helper()
	updated, cmd := s.Update(keyMsg(name))
	return Drive(t, updated, cmd)
}

// Resize sends a window-size message and settles.
func Resize(t *testing.T, s core.Screen, width, height int) core.Screen {
	t.Helper()
	updated, cmd := s.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return Drive(t, updated, cmd)
}

// CapturedCmd runs cmd and returns its message without feeding it back.
func CapturedCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// keyMsg treats anything not a named key as literal runes.
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

// Stub is a do-nothing core.Screen standing in for a drill-down target.
type Stub struct{}

func (Stub) Init() tea.Cmd                           { return nil }
func (s Stub) Update(tea.Msg) (core.Screen, tea.Cmd) { return s, nil }
func (Stub) View() string                            { return "stub" }
func (Stub) Title() string                           { return "stub" }
func (Stub) Keys() core.KeyMap                       { return core.KeyMap{} }
