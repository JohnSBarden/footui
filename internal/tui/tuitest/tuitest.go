// Package tuitest holds helpers for driving screens in tests without a
// terminal: Bubble Tea's Update/View are ordinary functions, so a screen
// can be stepped through its load sequence and its rendered output
// compared directly.
package tuitest

import (
	"testing"

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
// issuing commands.
func Drive(t *testing.T, s core.Screen, cmd tea.Cmd) core.Screen {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i >= maxSteps {
			t.Fatalf("tuitest: screen still issuing commands after %d steps", maxSteps)
		}
		msg := cmd()
		if msg == nil {
			break
		}
		s, cmd = s.Update(msg)
	}
	return s
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
