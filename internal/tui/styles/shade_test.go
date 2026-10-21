package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/JohnSBarden/footui/internal/colors"
)

// The shade comes back after every span ends, but a span with its own
// background (a form chip) keeps it.
func TestShadeSurvivesResetsAndLeavesOwnBackgrounds(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	bg := lipgloss.Color("#1C241F")
	shade := "\x1b[48;2;28;36;31m"
	chip := lipgloss.NewStyle().Background(lipgloss.Color("#FF0000")).Render("W")
	line := lipgloss.NewStyle().Bold(true).Render("Arsenal") + " " + chip + " end"

	got := Shade(line, bg)
	if !strings.HasPrefix(got, shade) {
		t.Fatalf("should open with the shade: %q", got)
	}
	// Once at the start, again after the bold name, after the chip.
	if n := strings.Count(got, shade); n != 3 {
		t.Errorf("shade set %d times, want 3: %q", n, got)
	}
	if !strings.Contains(got, "W") || strings.Contains(got, shade+"W") {
		t.Errorf("the chip's own background should be left alone: %q", got)
	}
}

// On a strong theme a selected panel's fill must not be its highlight
// (the text colour), and the text must read at least as well on it as on
// the stripe beneath.
func TestStrongShadeKeepsTextContrast(t *testing.T) {
	entries, err := colors.Load()
	if err != nil {
		t.Fatal(err)
	}
	themes := map[string]Theme{"base": BaseFor(Strong, false)}
	for _, e := range entries {
		if th, ok := StrongTeamTheme(e.Primary, e.Secondary); ok {
			themes[e.ID] = th
		}
	}
	for id, th := range themes {
		text, shade := concrete(t, th.Palette.Text), concrete(t, th.Shade)
		bg := concrete(t, th.Background)
		if got, floor := contrastRatio(text, shade), contrastRatio(text, bg); got < floor || got < minContrast {
			t.Errorf("%s: text contrast %.2f on shade %s, %.2f on the stripe", id, got, shade.Hex(), floor)
		}
	}
}

func TestShadeWithoutColourIsANoOp(t *testing.T) {
	if got := Shade("plain", lipgloss.Color("#1C241F")); got != "plain" {
		t.Errorf("got %q", got)
	}
}
