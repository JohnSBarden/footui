package components

import (
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func rows(n int) [][]Cell {
	out := make([][]Cell, n)
	for i := range out {
		out[i] = []Cell{Text(rowName(i))}
	}
	return out
}

func rowName(i int) string {
	return "row-" + itoa(i)
}

func table(t *testing.T, opts TableOpts) []string {
	t.Helper()
	if opts.Theme.TableCell.String() == "" {
		opts.Theme = styles.Plain()
	}
	return strings.Split(Table(opts), "\n")
}

func TestTableRendersHeaderAndEveryRowWhenItFits(t *testing.T) {
	lines := table(t, TableOpts{
		Columns: []Column{{Title: "Name", Width: 10}},
		Rows:    rows(3),
		Cursor:  -1,
	})

	if !strings.Contains(lines[0], "Name") {
		t.Errorf("first line should be the header, got %q", lines[0])
	}
	if len(lines) != 4 {
		t.Fatalf("expected a header plus 3 rows, got %d lines: %q", len(lines), lines)
	}
}

// The cursor must stay on screen, or arrowing down past the window would
// move a selection the user can no longer see.
func TestScrollingKeepsTheCursorVisible(t *testing.T) {
	const total, window = 40, 10

	for cursor := 0; cursor < total; cursor++ {
		out := Table(TableOpts{
			Columns: []Column{{Title: "Name", Width: 12}},
			Rows:    rows(total),
			Cursor:  cursor,
			MaxRows: window,
			Theme:   styles.Plain(),
		})
		want := rowName(cursor)
		if !strings.Contains(out, want) {
			t.Fatalf("cursor at %d scrolled %q off screen\n---\n%s", cursor, want, out)
		}
	}
}

func TestScrollingShowsExactlyMaxRows(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "Name", Width: 12}},
		Rows:    rows(40),
		Cursor:  20,
		MaxRows: 10,
		Theme:   styles.Plain(),
	})

	count := 0
	for i := 0; i < 40; i++ {
		if strings.Contains(out, rowName(i)+" ") || strings.HasSuffix(out, rowName(i)) {
			count++
		}
	}
	if count != 10 {
		t.Errorf("rendered %d rows, want the %d-row window", count, 10)
	}
}

// A user must not mistake a scrolled view for the whole league.
func TestATruncatedTableSaysHowMuchIsHidden(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "Name", Width: 12}},
		Rows:    rows(40),
		Cursor:  0,
		MaxRows: 10,
		Theme:   styles.Plain(),
	})

	if !strings.Contains(out, "of 40") {
		t.Errorf("expected a count of hidden rows\n---\n%s", out)
	}
}

func TestAnUntruncatedTableDoesNotClaimToBeScrolled(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "Name", Width: 12}},
		Rows:    rows(5),
		Cursor:  0,
		MaxRows: 10,
		Theme:   styles.Plain(),
	})

	if strings.Contains(out, "showing") {
		t.Errorf("a table that fits should not report a window\n---\n%s", out)
	}
}

func TestRowsShorterThanTheColumnsRenderBlanks(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "A", Width: 4}, {Title: "B", Width: 4}, {Title: "C", Width: 4}},
		Rows:    [][]Cell{{Text("x")}},
		Cursor:  -1,
		Theme:   styles.Plain(),
	})

	// A short row must not panic or drop the whole table.
	if !strings.Contains(out, "x") {
		t.Errorf("short row lost its content\n---\n%s", out)
	}
}

func TestNoColumnsRendersNothing(t *testing.T) {
	if got := Table(TableOpts{Rows: rows(3), Theme: styles.Plain()}); got != "" {
		t.Errorf("expected empty output with no columns, got %q", got)
	}
}

func TestEmptyTableStillRendersItsHeader(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "Name", Width: 10}},
		Cursor:  -1,
		Theme:   styles.Plain(),
	})
	if !strings.Contains(out, "Name") {
		t.Errorf("an empty table should still show its columns\n---\n%s", out)
	}
}

// Columns only line up if every cell is padded to the same display width,
// which is a different measure from byte length for accented names.
func TestMultiByteNamesAlignWithASCIIOnes(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "Team", Width: 20}, {Title: "Pts", Width: 4, Right: true}},
		Rows: [][]Cell{
			{Text("Arsenal"), Text("85")},
			{Text("Bayern München"), Text("78")},
			{Text("Beşiktaş"), Text("60")},
		},
		Cursor: -1,
		Theme:  styles.Plain(),
	})

	var widths []int
	for _, line := range strings.Split(out, "\n") {
		widths = append(widths, len([]rune(line)))
	}
	for i, w := range widths {
		if w != widths[0] {
			t.Errorf("line %d is %d cells wide, header is %d — columns won't line up\n---\n%s",
				i, w, widths[0], out)
		}
	}
}

func TestOverlongCellsAreTruncatedToTheColumnWidth(t *testing.T) {
	out := Table(TableOpts{
		Columns: []Column{{Title: "Team", Width: 8}},
		Rows:    [][]Cell{{Text("Wolverhampton Wanderers")}},
		Cursor:  -1,
		Theme:   styles.Plain(),
	})

	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 8 {
			t.Errorf("line %q exceeds the 8-cell column", line)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("a truncated cell should show it was cut\n---\n%s", out)
	}
}
