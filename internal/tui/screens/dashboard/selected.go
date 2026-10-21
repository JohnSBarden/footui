package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// The selected card's glint: a short run of bright border that travels
// clockwise around it, once every lapFrames frames.
const (
	lapFrames = 40
	glintLen  = 5
)

// selectedCard draws the selected card: a heavy border in the accent with
// the glint on it, around the card shaded in the theme's highlight.
// Without colour it is the plain heavy border, still.
func (m *Model) selectedCard(body string, inner int) string {
	th := m.deps.Theme
	box := lipgloss.NewStyle().Padding(0, 1).Width(inner + 2).Render(body)
	if th.NoColor {
		return lipgloss.NewStyle().Border(lipgloss.ThickBorder()).Render(box)
	}
	rows := strings.Split(box, "\n")
	w, h := inner+4, len(rows)
	perimeter := 2*w + 2*h
	head := m.frame * max(perimeter/lapFrames, 1) % perimeter
	// lit reports whether the border cell at clockwise index i, counted
	// from the top-left corner, is in the glint trailing head.
	lit := func(i int) bool { return (head-i+perimeter)%perimeter < glintLen }

	base := lipgloss.NewStyle().Foreground(th.Palette.Accent)
	glint := lipgloss.NewStyle().Foreground(th.Palette.Text).Bold(true)
	edge := func(glyphs []rune, index func(int) int) string {
		var b strings.Builder
		for start := 0; start < len(glyphs); {
			on := lit(index(start))
			end := start + 1
			for end < len(glyphs) && lit(index(end)) == on {
				end++
			}
			style := base
			if on {
				style = glint
			}
			b.WriteString(style.Render(string(glyphs[start:end])))
			start = end
		}
		return b.String()
	}
	cell := func(glyph rune, i int) string { return edge([]rune{glyph}, func(int) int { return i }) }

	bar := []rune("┏" + strings.Repeat("━", w-2) + "┓")
	out := make([]string, 0, h+2)
	out = append(out, edge(bar, func(x int) int { return x }))
	for y, row := range rows {
		out = append(out, cell('┃', 2*w+2*h-1-y)+styles.Shade(row, th.Shade)+cell('┃', w+y))
	}
	bar = []rune("┗" + strings.Repeat("━", w-2) + "┛")
	out = append(out, edge(bar, func(x int) int { return 2*w + h - 1 - x }))
	return strings.Join(out, "\n")
}
