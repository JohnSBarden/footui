package dashboard_test

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// L opens the log beside the cards: what's downloading and its latest
// progress, what's waiting, and what happened, newest first. L again, or
// the header's button, closes it.
func TestLogPanelShowsTheDownloadState(t *testing.T) {
	_, deps, _ := seed(t)
	deps.Log = core.NewLog(func() time.Time { return storagetest.Fixed })
	deps.Log.Add(core.LogDone, "MLS: 30 teams")
	deps.Log.Add(core.LogInfo, "start Premier League (2025)")
	deps.Log.Add(core.LogProgress, "season 2025: 380 matches")
	var s core.Screen = load(t, deps, 140)
	if !strings.Contains(s.View(), "≡ log") {
		t.Fatalf("the header should offer the log:\n%s", s.View())
	}
	s, _ = s.Update(core.RefreshQueueMsg{
		Active:  &core.RefreshRequest{CompetitionID: "eng-premier-league", Name: "Premier League"},
		Pending: []core.RefreshRequest{{CompetitionID: "usa-nwsl", Name: "NWSL"}},
	})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})

	view := s.View()
	for _, want := range []string{"× log", "NOW", "🔄 Premier League", "season 2025: 380 matches",
		"QUEUED 1", "· NWSL", "RECENT", "✓ MLS: 30 teams", "◆"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 140 {
			t.Errorf("line %d wide, past the screen: %q", w, line)
		}
	}
	// The newest entry comes first.
	if strings.Index(view, "season 2025") > strings.Index(view, "MLS: 30") {
		t.Error("recent entries should run newest first")
	}

	s, _ = s.Update(core.ClickMsg{ID: "log"})
	if strings.Contains(s.View(), "RECENT") {
		t.Error("the header's button should close the log")
	}
}

func TestLogPanelWhenIdle(t *testing.T) {
	_, deps, _ := seed(t)
	var s core.Screen = load(t, deps, 120)
	s, _ = s.Update(core.ClickMsg{ID: "log"})
	view := s.View()
	for _, want := range []string{"idle — nothing downloading", "nothing yet this session"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
}

// In colour, the selected card is shaded and a glint runs around its
// border, frame by frame; without colour nothing animates.
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
