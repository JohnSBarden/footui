package dashboard_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// The app draws the panel; the header's button follows its state.
func TestLogButtonFollowsThePanel(t *testing.T) {
	_, deps, _ := seed(t)
	var s core.Screen = load(t, deps, 140)
	if !strings.Contains(s.View(), "≡ log") {
		t.Fatalf("the header should offer the log:\n%s", s.View())
	}
	s, _ = s.Update(core.LogPanelMsg{Open: true})
	if !strings.Contains(s.View(), "× log") {
		t.Errorf("an open log should offer to close:\n%s", s.View())
	}
}

// Without colour nothing animates.
func TestSelectedCardGlintsInColour(t *testing.T) {
	_, deps, _ := seed(t)
	if load(t, deps, 120).Animating() {
		t.Error("a colourless dashboard shouldn't animate")
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	deps.Theme = styles.Default()
	m := load(t, deps, 120)
	if !m.Animating() {
		t.Fatal("the selected card should animate in colour")
	}
	first := m.View()
	if !strings.Contains(ansi.Strip(first), "┏") {
		t.Fatalf("the selected card should keep its heavy border:\n%s", ansi.Strip(first))
	}
	var s core.Screen = m
	for range 5 {
		s, _ = s.Update(core.FrameMsg{})
	}
	moved := s.View()
	if moved == first {
		t.Error("frames should move the glint")
	}
	if ansi.Strip(moved) != ansi.Strip(first) {
		t.Error("the glint should only recolour the border, not move anything")
	}
}
