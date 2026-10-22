package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// tallScreen renders more lines than any terminal has.
type tallScreen struct{ fakeScreen }

func (tallScreen) View() string {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "line"
	}
	lines[0] = "HEADER"
	return strings.Join(lines, "\n")
}

func (s tallScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	_, cmd := s.fakeScreen.Update(msg)
	return s, cmd
}

// A screen taller than the terminal loses its bottom, never its header.
func TestATallScreenIsClippedFromTheBottom(t *testing.T) {
	app := newApp(tallScreen{newFake("Team")})
	app = send(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	view := app.View()
	if n := strings.Count(view, "\n") + 1; n > 40 {
		t.Errorf("view is %d lines on a 40-line terminal", n)
	}
	if !strings.HasPrefix(view, "HEADER") {
		t.Errorf("the header must survive:\n%s", view[:min(len(view), 80)])
	}
	if !strings.Contains(view, "esc") {
		t.Error("the footer must survive too")
	}
}
