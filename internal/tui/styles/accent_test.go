package styles

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func useAccent(t *testing.T, spec string) {
	t.Helper()
	a, err := ParseAccent(spec)
	if err != nil {
		t.Fatal(err)
	}
	UseAccent(a)
	t.Cleanup(func() { UseAccent(Accent{}) })
}

func TestParseAccent(t *testing.T) {
	for in, want := range map[string]string{
		"": "lime", "Lime": "lime", "terminal": "terminal", "#FF8800": "#ff8800", "ff8800": "#ff8800",
	} {
		a, err := ParseAccent(in)
		if err != nil || a.String() != want {
			t.Errorf("ParseAccent(%q) = %v, %v; want %s", in, a, err, want)
		}
	}
	for _, bad := range []string{"purple", "#ff88", "#gg0000"} {
		if _, err := ParseAccent(bad); err == nil {
			t.Errorf("ParseAccent(%q) should fail", bad)
		}
	}
}

func TestAChosenAccentIsReadableOnBothBackgrounds(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	useAccent(t, "#ffd400") // a bright yellow, unreadable as-is on white
	p := Default().Palette
	a, ok := p.Accent.(lipgloss.AdaptiveColor)
	if !ok {
		t.Fatalf("accent is %T, want an adaptive colour", p.Accent)
	}
	if r := contrastRatio(mustHex(a.Light), mustHex(lightBackground)); r < minContrast {
		t.Errorf("light-terminal accent %s contrast %.2f", a.Light, r)
	}
	if r := contrastRatio(mustHex(a.Dark), mustHex(darkBackground)); r < minContrast {
		t.Errorf("dark-terminal accent %s contrast %.2f", a.Dark, r)
	}
	if mustHex(a.Dark).Hex() != "#ffd400" {
		t.Errorf("a colour that already reads on dark is kept, got %s", a.Dark)
	}
}

func TestTerminalAccentUsesTheTerminalsPalette(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	useAccent(t, "terminal")
	p := Default().Palette
	for name, c := range map[string]lipgloss.TerminalColor{
		"accent": p.Accent, "muted": p.Muted, "positive": p.Positive, "negative": p.Negative,
	} {
		if _, ok := c.(lipgloss.ANSIColor); !ok {
			t.Errorf("%s is %T, want an ANSI colour", name, c)
		}
	}
	if _, ok := p.Text.(lipgloss.NoColor); !ok {
		t.Errorf("text is %T, want the terminal's own foreground", p.Text)
	}
}

func TestStrongPitchWearsTheChosenAccent(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	useAccent(t, "#ff8800")
	lime := BaseFor(Strong, false).Palette.Accent
	if c, ok := lime.(lipgloss.Color); !ok || mustHex(string(c)).Hex() == baseLime {
		t.Errorf("strong mode kept lime: %v", lime)
	}
}
