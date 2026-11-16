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
	// Hint is an optional third line. A row where no card has one
	// collapses to two lines; otherwise every card keeps the third so the
	// row lines up.
	Hint string
	// Style optionally colours the value.
	Style *lipgloss.Style
}

// StatCards renders stats as bordered cards of one width, wrapping at
// maxWidth.
func StatCards(theme styles.Theme, stats []Stat, maxWidth int) string {
	if len(stats) == 0 {
		return ""
	}

	inner := cardInnerWidth(stats)
	cardTotal := inner + 4
	perRow := Balanced(len(stats), maxWidth/cardTotal)

	var lines []string
	for start := 0; start < len(stats); start += perRow {
		end := start + perRow
		if end > len(stats) {
			end = len(stats)
		}
		row := stats[start:end]
		hinted := false
		for _, s := range row {
			hinted = hinted || s.Hint != ""
		}
		rendered := make([]string, 0, len(row))
		for _, s := range row {
			rendered = append(rendered, renderCard(theme, s, inner, hinted))
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, rendered...))
	}

	return strings.Join(lines, "\n")
}

func renderCard(theme styles.Theme, s Stat, inner int, hinted bool) string {
	body := theme.Label.Render(pad(strings.ToUpper(s.Label), inner, false)) + "\n" +
		valueStyle(theme, s).Render(pad(s.Value, inner, false))
	if hinted {
		body += "\n" + theme.Muted.Render(pad(s.Hint, inner, false))
	}

	return theme.Panel.Render(body)
}

// cardInnerWidth is the widest content across the set.
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
