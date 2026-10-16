package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestBarUsesEighthBlocks(t *testing.T) {
	cases := []struct {
		v, max float64
		w      int
		want   string
	}{
		{0, 10, 5, ""},
		{10, 10, 5, "█████"},
		{5, 10, 5, "██▌"},
		{1, 10, 5, "▌"},
		{20, 10, 5, "█████"}, // clamped
		{-3, 10, 5, ""},
		{3, 0, 5, ""}, // no scale, no bar
	}
	for _, c := range cases {
		if got := Bar(c.v, c.max, c.w); got != c.want {
			t.Errorf("Bar(%v,%v,%d) = %q, want %q", c.v, c.max, c.w, got, c.want)
		}
	}
}

func TestDivergingPutsTheBarOnItsSide(t *testing.T) {
	neg, pos := Diverging(-5, 5, 4)
	if pos != "" || lipgloss.Width(neg) != 4 {
		t.Errorf("negative: %q %q", neg, pos)
	}
	neg, pos = Diverging(2.5, 5, 4)
	if neg != "" || pos != "██" {
		t.Errorf("positive: %q %q", neg, pos)
	}
}

func TestFormChipsKeepLettersWithoutColor(t *testing.T) {
	got := ansi.Strip(FormChips(styles.Plain(), "WDL"))
	if got != "W D L" {
		t.Errorf("plain chips = %q", got)
	}
	if ansi.Strip(FormChips(styles.Default(), "WL")) != " W  L " {
		t.Errorf("colour chips = %q", ansi.Strip(FormChips(styles.Default(), "WL")))
	}
	if FormChips(styles.Default(), "") != "" {
		t.Error("empty form renders nothing")
	}
}

func TestResultGlyphShapesDiffer(t *testing.T) {
	th := styles.Plain()
	got := ResultGlyph(th, 'W') + ResultGlyph(th, 'D') + ResultGlyph(th, 'L')
	if got != "●◐○" {
		t.Errorf("glyphs = %q", got)
	}
}

func TestDotFallsBackForUnknownColors(t *testing.T) {
	for _, hex := range []string{"", "not-a-color", "#DA291C"} {
		if ansi.Strip(Dot(styles.Default(), hex)) != "●" {
			t.Errorf("Dot(%q) lost its glyph", hex)
		}
	}
}

func TestScarfTopIsExactlyWidth(t *testing.T) {
	for _, w := range []int{2, 7, 38} {
		for _, th := range []styles.Theme{styles.Default(), styles.Plain()} {
			got := ScarfTop(th, "#DA291C", "#FBE122", w)
			if lipgloss.Width(got) != w {
				t.Errorf("width %d: got %d", w, lipgloss.Width(got))
			}
			if s := ansi.Strip(got); !strings.HasPrefix(s, "╭") || !strings.HasSuffix(s, "╮") {
				t.Errorf("corners missing: %q", s)
			}
		}
	}
}

func TestBlockDigitsAreThreeRowsOfEqualWidth(t *testing.T) {
	rows := BlockDigits("13")
	for i, r := range rows {
		if lipgloss.Width(r) != 7 { // 3 + gap + 3
			t.Errorf("row %d width %d: %q", i, lipgloss.Width(r), r)
		}
	}
	if BlockDigits("x")[0] != "" {
		t.Error("non-digits render nothing")
	}
}

func TestToneThresholds(t *testing.T) {
	th := styles.Default()
	if Tone(th, 21, 17.8).GetForeground() != th.Positive.GetForeground() {
		t.Error("beat expectation should be Positive")
	}
	if Tone(th, 9, 11.9).GetForeground() != th.Below.GetForeground() {
		t.Error("short of expectation should be Below")
	}
	if Tone(th, 3, 3.1).GetForeground() != th.TableCell.GetForeground() {
		t.Error("within 0.5 should be neutral")
	}
}

func TestLabelIsUpperCase(t *testing.T) {
	if got := ansi.Strip(Label(styles.Default(), "Recent")); got != "RECENT" {
		t.Errorf("Label = %q", got)
	}
	if !strings.Contains(ansi.Strip(Wordmark(styles.Default())), "● footui") {
		t.Error("wordmark")
	}
}
