package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Stat is one headline number on a dashboard.
type Stat struct {
	Label string
	Value string
	// Hint is an optional third line for context — a delta, a rank, or the
	// same figure per-90. Empty hints collapse the card to two lines.
	Hint string
	// Style optionally colors the value (e.g. green for over-performance).
	Style *lipgloss.Style
}

// StatCards renders stats as a row of bordered cards, wrapping onto
// further rows when they don't fit maxWidth. Cards share one width so the
// row reads as a grid rather than a ragged strip.
func StatCards(theme styles.Theme, stats []Stat, maxWidth int) string {
	if len(stats) == 0 {
		return ""
	}

	inner := cardInnerWidth(stats)
	// Each card costs its content plus the border and padding the theme's
	// Panel style adds on both sides.
	cardTotal := inner + 4
	perRow := maxWidth / cardTotal
	if perRow < 1 {
		perRow = 1
	}

	var lines []string
	for start := 0; start < len(stats); start += perRow {
		end := start + perRow
		if end > len(stats) {
			end = len(stats)
		}
		rendered := make([]string, 0, end-start)
		for _, s := range stats[start:end] {
			rendered = append(rendered, renderCard(theme, s, inner))
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
	}

	return strings.Join(lines, "\n")
}

func renderCard(theme styles.Theme, s Stat, inner int) string {
	valueStyle := theme.Accent.Bold(true)
	if s.Style != nil {
		valueStyle = s.Style.Bold(true)
	}

	body := theme.Label.Render(pad(strings.ToUpper(s.Label), inner, false)) + "\n" +
		valueStyle.Render(pad(s.Value, inner, false))
	if s.Hint != "" {
		body += "\n" + theme.Muted.Render(pad(s.Hint, inner, false))
	}

	return theme.Panel.Render(body)
}

// cardInnerWidth sizes every card to the widest content across the set, so
// the labels and values line up column-wise down a wrapped grid.
func cardInnerWidth(stats []Stat) int {
	const min = 7
	w := min
	for _, s := range stats {
		for _, text := range []string{s.Label, s.Value, s.Hint} {
			if n := lipgloss.Width(text); n > w {
				w = n
			}
		}
	}
	return w
}
