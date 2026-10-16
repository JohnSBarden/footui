package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestHeroIsFullWidthWithRule(t *testing.T) {
	fill, rule := styles.HeroStyles(styles.Default(), "#DA291C", "#FBE122", styles.Weak)
	out := Hero(HeroOpts{Fill: fill, Rule: rule, Width: 80, Title: "Manchester United",
		Meta: "England · men", Right: []string{"2026-27", "13th · 6 pts"}})
	lines := strings.Split(out, "\n")
	if len(lines) != 3 { // 2 content rows + rule
		t.Fatalf("lines = %d:\n%s", len(lines), out)
	}
	for i, l := range lines {
		if lipgloss.Width(l) != 80 {
			t.Errorf("line %d width %d: %q", i, lipgloss.Width(l), ansi.Strip(l))
		}
	}
	if !strings.Contains(ansi.Strip(lines[1]), "13th · 6 pts") {
		t.Errorf("right slot missing: %q", ansi.Strip(lines[1]))
	}
}

func TestHeroDropsTheRightSlotWhenNarrow(t *testing.T) {
	out := Hero(HeroOpts{Width: 40, Title: "Borussia Mönchengladbach", Meta: "Germany",
		Right: []string{"2026-27", "13th · 6 pts"}, NoColor: true})
	for _, l := range strings.Split(out, "\n") {
		if lipgloss.Width(l) > 40 {
			t.Errorf("overflow: %q", l)
		}
	}
	if !strings.Contains(ansi.Strip(out), "13th") {
		t.Error("right slot must move below, not vanish")
	}
}

func TestHeroTruncatesAnOverlongTitle(t *testing.T) {
	out := Hero(HeroOpts{Width: 80, Title: strings.Repeat("Long name ", 12), Meta: "x",
		Right: []string{"2026-27"}, NoColor: true})
	for _, l := range strings.Split(out, "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("overflow %d: %q", lipgloss.Width(l), l)
		}
	}
}

func TestHeroStylesSurviveABadHex(t *testing.T) {
	fill, _ := styles.HeroStyles(styles.Default(), "nope", "", styles.Weak)
	if fill.GetBackground() == (lipgloss.NoColor{}) {
		t.Error("bad hex should fall back to the accent band")
	}
}
