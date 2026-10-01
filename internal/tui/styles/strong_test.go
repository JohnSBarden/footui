package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"

	"github.com/JohnSBarden/footui/internal/colors"
)

func concrete(t *testing.T, c lipgloss.TerminalColor) colorful.Color {
	t.Helper()
	v, ok := c.(lipgloss.Color)
	if !ok {
		t.Fatalf("strong theme color %v is not concrete", c)
	}
	return mustHex(string(v))
}

// Every curated club must come out readable when its own primary is the
// background — the property that makes a strong theme usable at all.
func TestStrongThemeIsReadableForEveryCuratedClub(t *testing.T) {
	entries, err := colors.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		th, ok := StrongTeamTheme(e.Primary, e.Secondary)
		if !ok {
			t.Errorf("%s: no strong theme for primary %q", e.ID, e.Primary)
			continue
		}
		bg := concrete(t, th.Background)
		for name, c := range map[string]lipgloss.TerminalColor{
			"text": th.Palette.Text, "muted": th.Palette.Muted, "accent": th.Palette.Accent,
		} {
			if r := contrastRatio(concrete(t, c), bg); r < minContrast {
				t.Errorf("%s: %s contrast %.2f on %s, want ≥ %.1f", e.ID, name, r, e.Primary, minContrast)
			}
		}
		// Green/red keep their hue, so on a club whose primary is itself
		// green or red the reachable contrast is lower; large-text AA is
		// the floor.
		for name, c := range map[string]lipgloss.TerminalColor{
			"positive": th.Palette.Positive, "negative": th.Palette.Negative,
		} {
			if r := contrastRatio(concrete(t, c), bg); r < minLargeContrast {
				t.Errorf("%s: %s contrast %.2f on %s, want ≥ %.1f", e.ID, name, r, e.Primary, minLargeContrast)
			}
		}
	}
}

func TestStrongThemeFallsBackToWeakWithoutAPrimary(t *testing.T) {
	if _, ok := StrongTeamTheme("", "#ffffff"); ok {
		t.Error("no primary, no strong theme")
	}
	if th := TeamThemeFor("", "", Strong); th.Background != nil {
		t.Error("the fallback must leave the terminal background alone")
	}
	if th := TeamThemeFor("#EF0107", "#FFFFFF", Weak); th.Background != nil {
		t.Error("weak mode must not paint a background")
	}
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"": Weak, "weak": Weak, "STRONG": Strong, " strong ": Strong} {
		if got, err := ParseMode(in); err != nil || got != want {
			t.Errorf("ParseMode(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseMode("loud"); err == nil {
		t.Error("an unknown mode should be an error")
	}
}

func TestPaintFillsEveryCellWithTheBackground(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	th, _ := StrongTeamTheme("#EF0107", "#FFFFFF")
	frame := "plain " + th.Accent.Render("styled") + " tail\nshort"
	out := Paint(frame, th, 20, 3)

	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want the frame padded to 3", len(lines))
	}
	bg := termenv.TrueColor.Color("#ef0107").Sequence(true)
	for i, line := range lines {
		if !strings.Contains(line, bg) {
			t.Errorf("line %d has no background: %q", i, line)
		}
		if w := lipgloss.Width(line); w != 20 {
			t.Errorf("line %d is %d wide, want padded to 20", i, w)
		}
	}
	// After the styled span's reset the background must come straight back.
	if !strings.Contains(lines[0], reset+termenv.CSI) {
		t.Errorf("background not re-applied after a reset: %q", lines[0])
	}
}

func TestPaintLeavesTransparentThemesAlone(t *testing.T) {
	frame := "a\nb"
	if got := Paint(frame, Default(), 10, 5); got != frame {
		t.Errorf("a theme without a background must not be painted, got %q", got)
	}
}
