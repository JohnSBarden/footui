package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// stripes are the two mowing shades a strong theme alternates between.
func stripes(t *testing.T, th Theme) []colorful.Color {
	t.Helper()
	return []colorful.Color{concrete(t, th.Background), concrete(t, th.Stripe)}
}

// Every curated club must be readable on its own primary.
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
		if th.Stripe == nil || colorHex(th.Stripe) == colorHex(th.Background) {
			t.Errorf("%s: no second mowing stripe", e.ID)
		}
		for _, bg := range stripes(t, th) {
			for name, c := range map[string]lipgloss.TerminalColor{
				"text": th.Palette.Text, "muted": th.Palette.Muted,
			} {
				if r := contrastRatio(concrete(t, c), bg); r < minContrast {
					t.Errorf("%s: %s contrast %.2f on %s, want ≥ %.1f", e.ID, name, r, bg.Hex(), minContrast)
				}
			}
			// The accent may fall back to bold text, which only needs
			// large-text AA.
			want := minContrast
			if th.Accent.GetBold() {
				want = minLargeContrast
			}
			if r := contrastRatio(concrete(t, th.Palette.Accent), bg); r < want {
				t.Errorf("%s: accent contrast %.2f on %s, want ≥ %.1f", e.ID, r, bg.Hex(), want)
			}
		}
		// Green/red keep their hue, so on a green or red primary
		// large-text AA is the floor.
		for _, bg := range stripes(t, th) {
			for name, c := range map[string]lipgloss.TerminalColor{
				"positive": th.Palette.Positive, "negative": th.Palette.Negative,
			} {
				if r := contrastRatio(concrete(t, c), bg); r < minLargeContrast {
					t.Errorf("%s: %s contrast %.2f on %s, want ≥ %.1f", e.ID, name, r, bg.Hex(), minLargeContrast)
				}
			}
		}
	}
}

// On a bright red neither text colour nor a navy secondary reaches 4.5:1;
// the accent must still stand apart from the body text.
func TestStrongAccentDiffersFromTextOnBrightRed(t *testing.T) {
	th, ok := StrongTeamTheme("#EF0107", "#023474") // Arsenal
	if !ok {
		t.Fatal("no strong theme")
	}
	accent, text := concrete(t, th.Palette.Accent), concrete(t, th.Palette.Text)
	if accent == text {
		t.Fatalf("accent %s is the body text colour", accent.Hex())
	}
	for _, bg := range stripes(t, th) {
		r := contrastRatio(accent, bg)
		if r < minLargeContrast {
			t.Errorf("accent contrast %.2f on %s, want ≥ %.1f", r, bg.Hex(), minLargeContrast)
		}
		if r < minContrast && !th.Accent.GetBold() {
			t.Errorf("accent contrast %.2f on %s is below %.1f but not bold", r, bg.Hex(), minContrast)
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

// Screens with no club (Browse, a league table) must change with the mode
// too, or switching looks like it did nothing.
func TestBaseThemeFollowsTheMode(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if BaseFor(Weak, false).Background != nil {
		t.Error("weak mode keeps the terminal's background")
	}
	strong := BaseFor(Strong, false)
	if strong.Background == nil {
		t.Fatal("strong mode paints club-less screens in footui's colors")
	}
	for _, bg := range stripes(t, strong) {
		for name, c := range map[string]lipgloss.TerminalColor{
			"text": strong.Palette.Text, "muted": strong.Palette.Muted, "accent": strong.Palette.Accent,
		} {
			if r := contrastRatio(concrete(t, c), bg); r < minContrast {
				t.Errorf("footui's own strong theme: %s contrast %.2f on %s, want ≥ %.1f", name, r, bg.Hex(), minContrast)
			}
		}
	}
	if !BaseFor(Strong, true).NoColor {
		t.Error("--no-color wins over strong mode")
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

// Styles that draw their own background (keycaps, tab pills, title,
// selected row) must read against it; the palette check can't see these.
func TestStrongThemeFilledStylesAreReadable(t *testing.T) {
	entries, err := colors.Load()
	if err != nil {
		t.Fatal(err)
	}
	themes := map[string]Theme{"footui": BaseFor(Strong, false)}
	for _, e := range entries {
		if th, ok := StrongTeamTheme(e.Primary, e.Secondary); ok {
			themes[e.ID] = th
		}
	}
	t.Setenv("NO_COLOR", "")
	for id, th := range themes {
		for name, s := range map[string]lipgloss.Style{
			"keycap": th.Keycap, "tab active": th.TabActive, "tab idle": th.TabIdle,
			"title": th.Title, "selected row": th.TableSelected,
		} {
			bgs := stripes(t, th)
			if bg := s.GetBackground(); bg != (lipgloss.NoColor{}) {
				bgs = []colorful.Color{concrete(t, bg)}
			}
			for _, bg := range bgs {
				if r := contrastRatio(concrete(t, s.GetForeground()), bg); r < minContrast {
					t.Errorf("%s: %s contrast %.2f on %s, want ≥ %.1f", id, name, r, bg.Hex(), minContrast)
				}
			}
		}
	}
}

// Cells with no background of their own take the mowing stripe for their
// column; a span with its own background (a keycap) keeps it throughout.
func TestPaintMowsStripesBehindUnfilledCells(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	th, _ := StrongTeamTheme("#EF0107", "#FFFFFF")
	a := termenv.TrueColor.Color(colorHex(th.Background)).Sequence(true)
	b := termenv.TrueColor.Color(colorHex(th.Stripe)).Sequence(true)
	chip := lipgloss.NewStyle().Background(lipgloss.Color("#000000"))
	frame := "abcd" + chip.Render("efghij") + "klmnopqrstu"
	out := Paint(frame, th, 24, 1)

	if got := ansi.Strip(out); got != "abcdefghijklmnopqrstu   " {
		t.Fatalf("text changed: %q", got)
	}
	if !strings.Contains(out, a) || !strings.Contains(out, b) {
		t.Fatalf("want both stripes in %q", out)
	}
	// The chip's own background runs unbroken across the stripe boundary
	// at column 6, and the stripe after it resumes on the second shade.
	if !strings.Contains(out, "efghij") {
		t.Errorf("chip interrupted by a stripe: %q", out)
	}
	after := out[strings.Index(out, "efghij")+len("efghij"):]
	if i, j := strings.Index(after, b), strings.Index(after, "kl"); i < 0 || i > j {
		t.Errorf("column 10 should be back on the second stripe: %q", after)
	}
	// Column 12 starts the first shade again.
	if i, j := strings.Index(after, a), strings.Index(after, "mnop"); i < 0 || i > j || i < strings.Index(after, "kl") {
		t.Errorf("column 12 should switch to the first stripe: %q", after)
	}
}
