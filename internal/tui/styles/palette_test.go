package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDefaultPaletteTextRolesAreReadable(t *testing.T) {
	p := DefaultPalette()
	roles := map[string]lipgloss.TerminalColor{
		"Accent": p.Accent, "Text": p.Text, "Muted": p.Muted,
		"Positive": p.Positive, "Negative": p.Negative,
		"Draw": p.Draw, "Info": p.Info, "Below": p.Below,
	}
	for name, c := range roles {
		ac, ok := c.(lipgloss.AdaptiveColor)
		if !ok {
			t.Fatalf("%s is not adaptive", name)
		}
		for bg, fg := range map[string]string{darkBackground: ac.Dark, lightBackground: ac.Light} {
			if r := contrastRatio(mustHex(fg), mustHex(bg)); r < minContrast {
				t.Errorf("%s %s on %s: %.2f < %.1f", name, fg, bg, r, minContrast)
			}
		}
	}
}

func TestDefaultAccentIsPurple(t *testing.T) {
	ac := DefaultPalette().Accent.(lipgloss.AdaptiveColor)
	if ac.Dark != "#A66BFF" || ac.Light != "#6D28D9" {
		t.Fatalf("accent = %+v", ac)
	}
}

func TestStrongBaseIsViolet(t *testing.T) {
	th := BaseFor(Strong, false)
	if !strings.EqualFold(colorHex(th.Background), "#2E1065") {
		t.Fatalf("strong base background = %q", colorHex(th.Background))
	}
}
