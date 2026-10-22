// Package components holds stateless render helpers shared by screens:
// they take a styles.Theme and return a string.
package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Column describes one table column.
type Column struct {
	Title string
	Width int
	// Right right-aligns the column (use for numbers).
	Right bool
}

// Cell is one table cell. Style overrides the theme's cell style except
// on the selected row.
type Cell struct {
	Text  string
	Style *lipgloss.Style
}

// Text builds an unstyled cell.
func Text(s string) Cell { return Cell{Text: s} }

// Styled builds a cell with an explicit style.
func Styled(s string, style lipgloss.Style) Cell { return Cell{Text: s, Style: &style} }

// TableOpts configures Table.
type TableOpts struct {
	Columns []Column
	Rows    [][]Cell
	// Cursor is the index of the selected row, or -1 for no selection.
	Cursor int
	// MaxRows caps rendered rows, scrolling to keep Cursor visible; 0 means
	// all.
	MaxRows int
	Theme   styles.Theme
	// Zone, when set, makes each row clickable as Zone+":"+its index.
	Zone string
}

// Table renders a fixed-width column table with an optional selected row.
func Table(opts TableOpts) string {
	if len(opts.Columns) == 0 {
		return ""
	}

	var b strings.Builder

	header := make([]string, len(opts.Columns))
	for i, c := range opts.Columns {
		header[i] = pad(c.Title, c.Width, c.Right)
	}
	b.WriteString(opts.Theme.TableHeader.Render(strings.Join(header, " ")))
	b.WriteString("\n")

	start, end := visibleWindow(len(opts.Rows), opts.Cursor, opts.MaxRows)
	for i := start; i < end; i++ {
		row := renderRow(opts, opts.Rows[i], i == opts.Cursor)
		if opts.Zone != "" {
			row = Zone(opts.Zone+":"+itoa(i), row)
		}
		b.WriteString(row)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	if end < len(opts.Rows) || start > 0 {
		b.WriteString("\n")
		b.WriteString(opts.Theme.Muted.Render(
			"  showing " + itoa(start+1) + "-" + itoa(end) + " of " + itoa(len(opts.Rows))))
	}

	return b.String()
}

func renderRow(opts TableOpts, row []Cell, selected bool) string {
	parts := make([]string, len(opts.Columns))
	for i, col := range opts.Columns {
		var cell Cell
		if i < len(row) {
			cell = row[i]
		}
		text := pad(cell.Text, col.Width, col.Right)

		switch {
		case selected:
			// Per-cell colours are dropped: mixed with the selection
			// background they're unreadable on some terminals.
			parts[i] = text
		case cell.Style != nil:
			parts[i] = cell.Style.Render(text)
		default:
			parts[i] = opts.Theme.TableCell.Render(text)
		}
	}

	line := strings.Join(parts, " ")
	if selected {
		return opts.Theme.TableSelected.Render(line)
	}
	return line
}

// visibleWindow returns the [start, end) row range keeping cursor inside.
func visibleWindow(total, cursor, maxRows int) (int, int) {
	if maxRows <= 0 || total <= maxRows {
		return 0, total
	}
	start := 0
	if cursor >= 0 {
		// Keep the cursor centred past the halfway point.
		start = cursor - maxRows/2
	}
	if start < 0 {
		start = 0
	}
	if start > total-maxRows {
		start = total - maxRows
	}
	return start, start + maxRows
}

// pad truncates or pads s to exactly width display cells (not bytes).
func pad(s string, width int, right bool) string {
	if width <= 0 {
		return s
	}
	if lipgloss.Width(s) > width {
		s = Truncate(s, width)
	}
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// Truncate shortens s to at most width display cells, ending in "…".
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(s)
	for len(runes) > 0 {
		candidate := string(runes) + "…"
		if lipgloss.Width(candidate) <= width {
			return candidate
		}
		runes = runes[:len(runes)-1]
	}
	return "…"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
