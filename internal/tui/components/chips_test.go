package components

import (
	"math"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// On a red strong theme the win colour is solved to near-black; dark chip
// text on it must not disappear.
func TestFormChipLettersReadOnStrongThemes(t *testing.T) {
	th, ok := styles.StrongTeamTheme("#EF0107", "#FFFFFF")
	if !ok {
		t.Fatal("no strong theme")
	}
	for _, r := range []byte("WDL") {
		s := chipStyle(th, r)
		fg, _ := colorful.Hex(string(s.GetForeground().(lipgloss.Color)))
		bg, _ := colorful.Hex(string(s.GetBackground().(lipgloss.Color)))
		if l1, l2 := luminance(fg), luminance(bg); (max(l1, l2)+0.05)/(min(l1, l2)+0.05) < 4.5 {
			t.Errorf("%c chip: %s on %s is unreadable", r, fg.Hex(), bg.Hex())
		}
	}
}

// luminance is WCAG relative luminance.
func luminance(c colorful.Color) float64 {
	ch := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}
