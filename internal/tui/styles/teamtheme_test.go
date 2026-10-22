package styles

import (
	"math"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

func hex(t *testing.T, s string) colorful.Color {
	t.Helper()
	c, err := colorful.Hex(s)
	if err != nil {
		t.Fatalf("bad test color %q: %v", s, err)
	}
	return c
}

func TestContrastRatioMatchesTheWCAGExtremes(t *testing.T) {
	black, white := hex(t, "#000000"), hex(t, "#ffffff")

	if got := contrastRatio(black, white); math.Abs(got-21) > 0.01 {
		t.Errorf("black on white = %v, want 21", got)
	}
	if got := contrastRatio(white, white); math.Abs(got-1) > 0.01 {
		t.Errorf("white on white = %v, want 1", got)
	}
	// Ratio is symmetric — the order of the arguments must not matter.
	if contrastRatio(black, white) != contrastRatio(white, black) {
		t.Error("contrast ratio should be symmetric")
	}
}

// A club colour must end up readable on either terminal background.
func TestBrandColorsBecomeReadableOnBothBackgrounds(t *testing.T) {
	// Real brand colours chosen to be hard cases at each end.
	cases := map[string]string{
		"Newcastle black":   "#241F20",
		"Norwich yellow":    "#FFF200",
		"Real Madrid white": "#FEBE10",
		"Chelsea blue":      "#034694",
		"Leeds white":       "#FFFFFF",
	}

	light, dark := hex(t, lightBackground), hex(t, darkBackground)

	for name, brand := range cases {
		c := hex(t, brand)

		onLight := adjustForBackground(c, light, minContrast)
		if got := contrastRatio(onLight, light); got < minContrast {
			t.Errorf("%s on a light terminal: contrast %.2f, want >= %.1f", name, got, minContrast)
		}

		onDark := adjustForBackground(c, dark, minContrast)
		if got := contrastRatio(onDark, dark); got < minContrast {
			t.Errorf("%s on a dark terminal: contrast %.2f, want >= %.1f", name, got, minContrast)
		}
	}
}

// Adjusting must move lightness only, keeping the club's hue.
func TestAdjustingPreservesHue(t *testing.T) {
	chelsea := hex(t, "#034694")
	wantHue, _, _ := chelsea.Hsl()

	for _, bg := range []string{lightBackground, darkBackground} {
		adjusted := adjustForBackground(chelsea, hex(t, bg), minContrast)
		gotHue, _, _ := adjusted.Hsl()
		if math.Abs(gotHue-wantHue) > 1.0 {
			t.Errorf("hue drifted from %.1f to %.1f adjusting for %s", wantHue, gotHue, bg)
		}
	}
}

// A colour that is already readable must be left exactly as the club chose.
func TestAlreadyReadableColorsAreNotTouched(t *testing.T) {
	// A mid-tone red clears 4.5:1 against white already.
	c := hex(t, "#8B0000")
	got := adjustForBackground(c, hex(t, lightBackground), minContrast)
	if got.Hex() != c.Hex() {
		t.Errorf("adjusted %s to %s despite it already meeting contrast", c.Hex(), got.Hex())
	}
}

func TestTextOnBrandColorPicksTheReadableEnd(t *testing.T) {
	cases := map[string]string{
		"#241F20": "#ffffff", // near-black club colour needs white text
		"#FFF200": "#000000", // bright yellow needs black text
		"#FFFFFF": "#000000",
		"#034694": "#ffffff",
	}
	for brand, want := range cases {
		got := readableTextOn(hex(t, brand))
		if got != lipgloss.Color(want) {
			t.Errorf("text on %s = %v, want %s", brand, got, want)
		}
	}
}

func TestUnparseableBrandColorFallsBackToTheNeutralPalette(t *testing.T) {
	for _, bad := range []string{"", "not-a-color", "#12345"} {
		got := TeamPalette(bad, "")
		if got.Accent != DefaultPalette().Accent {
			t.Errorf("TeamPalette(%q) should fall back to the default palette, got %+v", bad, got.Accent)
		}
	}
}

func TestTeamPaletteUsesTheBrandColor(t *testing.T) {
	got := TeamPalette("#DC052D", "#0066B2")

	if got.Accent == DefaultPalette().Accent {
		t.Error("a valid brand color should replace the default accent")
	}
	if got.Border == DefaultPalette().Border {
		t.Error("a valid secondary color should replace the default border")
	}
}

// Plain() must not leak any club colour.
func TestPlainThemeCarriesNoColor(t *testing.T) {
	if !Plain().NoColor {
		t.Error("Plain() should report NoColor")
	}
	if Resolve(true).NoColor != true {
		t.Error("--no-color should resolve to the plain theme")
	}
}
