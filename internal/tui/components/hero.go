package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// heroMinWidth is the narrowest terminal that keeps the band's right slot
// beside the title; narrower, it moves underneath.
const heroMinWidth = 80

// heroMinTitle is the title width kept before dropping the right text.
const heroMinTitle = 12

// HeroOpts configures Hero.
type HeroOpts struct {
	Fill, Rule lipgloss.Style
	Width      int
	Title      string   // bold, left
	Meta       string   // second line, left
	Right      []string // right-aligned lines, top to bottom
	NoColor    bool
}

// Hero is a club or player screen's header band and rule.
func Hero(o HeroOpts) string {
	left := []string{o.Fill.Bold(true).Render(o.Title), o.Fill.Render(o.Meta)}
	right := o.Right
	var below []string
	if o.Width < heroMinWidth {
		below, right = right, nil
	}
	// One cell of margin on the left, two on the right.
	avail := max(o.Width-3, 0)
	rows := max(len(left), len(right))
	lines := make([]string, 0, rows+1+len(below))
	for i := 0; i < rows; i++ {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = o.Fill.Render(right[i])
		}
		maxL := avail
		if r != "" {
			maxL = avail - lipgloss.Width(r) - 1
			if maxL < heroMinTitle {
				r, maxL = "", avail
			}
		}
		l = ansi.Truncate(l, maxL, "…")
		gap := max(avail-lipgloss.Width(l)-lipgloss.Width(r), 0)
		lines = append(lines, o.Fill.Render(" ")+l+o.Fill.Render(strings.Repeat(" ", gap))+r+o.Fill.Render("  "))
	}
	ruleGlyph := "▀"
	if o.NoColor {
		ruleGlyph = "─"
	}
	lines = append(lines, o.Rule.Render(strings.Repeat(ruleGlyph, max(o.Width, 0))))
	for _, b := range below {
		lines = append(lines, ansi.Truncate(" "+b, o.Width, "…"))
	}
	return strings.Join(lines, "\n")
}
