package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Screens measure what they've drawn rather than hard-coding line counts,
// which go stale the moment a header grows.

// Lines is how many terminal lines s takes; none for an empty string.
func Lines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// TableRows is how many of total rows a Table can show in lines,
// allowing for the header and the "showing x-y of z" line.
func TableRows(lines, total int) int {
	if total <= lines-1 {
		return max(total, 1)
	}
	return max(lines-2, 1)
}

// TableLines is the inverse of TableRows.
func TableLines(rows, total int) int {
	if total <= rows {
		return max(total, 1) + 1
	}
	return rows + 2
}

// SideBySide sets blocks next to each other, padded to widths. Long lines
// are cut, not wrapped, so rows stay in step across columns.
func SideBySide(gap int, widths []int, blocks ...string) string {
	cols := make([][]string, len(blocks))
	height := 0
	for i, b := range blocks {
		cols[i] = strings.Split(b, "\n")
		height = max(height, len(cols[i]))
	}
	spacer := strings.Repeat(" ", gap)
	out := make([]string, height)
	for row := range out {
		var b strings.Builder
		for i, col := range cols {
			line := ""
			if row < len(col) {
				line = col[row]
			}
			if i == len(cols)-1 {
				// The last column's right edge is the screen's: no padding.
				b.WriteString(ansi.Truncate(line, widths[i], "…"))
				break
			}
			line = ansi.Truncate(line, widths[i], "…")
			b.WriteString(line + strings.Repeat(" ", widths[i]-lipgloss.Width(line)) + spacer)
		}
		out[row] = strings.TrimRight(b.String(), " ")
	}
	return strings.Join(out, "\n")
}

// StatPanel renders stats in the roomiest form that fits in maxLines:
// cards, borderless cards, then a line each. Only then are the last stats
// dropped, so screens list them most telling first.
func StatPanel(th styles.Theme, stats []Stat, width, maxLines int) string {
	if len(stats) == 0 {
		return ""
	}
	if cards := StatCards(th, stats, width); Lines(cards) <= maxLines {
		return cards
	}
	if flat := flatCards(th, stats, width); Lines(flat) <= maxLines {
		return flat
	}
	lines := strings.Split(StatLines(th, stats, width), "\n")
	return strings.Join(lines[:min(len(lines), max(maxLines, 1))], "\n")
}

// flatCards are stat cards without the border and padding.
func flatCards(th styles.Theme, stats []Stat, width int) string {
	inner := cardInnerWidth(stats)
	perRow := Balanced(len(stats), (width+2)/(inner+2))
	var rows []string
	for start := 0; start < len(stats); start += perRow {
		end := min(start+perRow, len(stats))
		lines := make([]string, 3)
		hints := false
		for i, s := range stats[start:end] {
			sep := ""
			if i > 0 {
				sep = "  "
			}
			lines[0] += sep + th.Label.Render(pad(strings.ToUpper(s.Label), inner, false))
			lines[1] += sep + valueStyle(th, s).Render(pad(s.Value, inner, false))
			lines[2] += sep + th.Muted.Render(pad(s.Hint, inner, false))
			hints = hints || s.Hint != ""
		}
		if !hints {
			lines = lines[:2]
		}
		rows = append(rows, strings.Join(lines, "\n"))
	}
	return strings.Join(rows, "\n")
}

// StatLines puts each stat on a line of its own, packed into as many
// columns as width allows.
func StatLines(th styles.Theme, stats []Stat, width int) string {
	labelW, valueW, hintW := 0, 0, 0
	for _, s := range stats {
		labelW = max(labelW, lipgloss.Width(s.Label))
		valueW = max(valueW, lipgloss.Width(s.Value))
		hintW = max(hintW, lipgloss.Width(s.Hint))
	}
	cell := labelW + 1 + valueW
	if hintW > 0 {
		cell += 1 + hintW
	}
	const gap = 3
	perRow := Balanced(len(stats), (width+gap)/(cell+gap))
	var lines []string
	for start := 0; start < len(stats); start += perRow {
		end := min(start+perRow, len(stats))
		var b strings.Builder
		for i, s := range stats[start:end] {
			text := th.Label.Render(pad(strings.ToUpper(s.Label), labelW, false)) + " " +
				valueStyle(th, s).Render(pad(s.Value, valueW, false))
			if hintW > 0 {
				text += " " + th.Muted.Render(pad(s.Hint, hintW, false))
			}
			if i > 0 {
				b.WriteString(strings.Repeat(" ", gap))
			}
			b.WriteString(text)
		}
		lines = append(lines, ansi.Truncate(strings.TrimRight(b.String(), " "), width, "…"))
	}
	return strings.Join(lines, "\n")
}

func valueStyle(th styles.Theme, s Stat) lipgloss.Style {
	if s.Style != nil {
		return s.Style.Bold(true)
	}
	return th.Accent.Bold(true)
}

// ColumnsWidth is the width a Table with these columns draws.
func ColumnsWidth(cols []Column) int {
	w := 0
	for _, c := range cols {
		w += c.Width + 1
	}
	return max(w-1, 0)
}

// Balanced is how many of n items to put per row, at most perRow, filled
// evenly (ten wraps five and five, not eight and two).
func Balanced(n, perRow int) int {
	perRow = max(perRow, 1)
	if n <= perRow {
		return max(n, 1)
	}
	rows := (n + perRow - 1) / perRow
	return (n + rows - 1) / rows
}
