package components

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestBalancedFillsRowsEvenly(t *testing.T) {
	cases := []struct{ n, perRow, want int }{
		{10, 8, 5}, // 5 and 5, not 8 and a stray 2
		{3, 8, 3},
		{7, 3, 3},
		{9, 3, 3},
		{4, 0, 1}, // nothing fits: one a row
	}
	for _, c := range cases {
		if got := Balanced(c.n, c.perRow); got != c.want {
			t.Errorf("Balanced(%d, %d) = %d, want %d", c.n, c.perRow, got, c.want)
		}
	}
}

// A table given TableRows(lines) rows must draw in those lines, header and
// "showing" line included, or the screen runs past its footer.
func TestTableRowsFitTheirLines(t *testing.T) {
	th := styles.Plain()
	cols := []Column{{Title: "N", Width: 3}}
	for _, total := range []int{1, 3, 10, 40} {
		rows := make([][]Cell, total)
		for i := range rows {
			rows[i] = []Cell{Text(strconv.Itoa(i))}
		}
		for lines := 3; lines < 45; lines++ {
			n := TableRows(lines, total)
			out := Table(TableOpts{Columns: cols, Rows: rows, Cursor: 0, MaxRows: n, Theme: th})
			if got := Lines(out); got > lines {
				t.Errorf("total %d in %d lines: drew %d", total, lines, got)
			}
			if got, want := TableLines(n, total), Lines(out); got != want {
				t.Errorf("TableLines(%d, %d) = %d, table drew %d", n, total, got, want)
			}
		}
	}
}

func tenStats() []Stat {
	var out []Stat
	for i := range 10 {
		out = append(out, Stat{Label: "Stat " + strconv.Itoa(i), Value: strconv.Itoa(i * 7), Hint: "hint"})
	}
	return out
}

// With room the numbers keep their boxes; short of it, the boxes go
// before any number does, and only one line each loses stats.
func TestStatPanelGivesUpBoxesBeforeNumbers(t *testing.T) {
	th := styles.Plain()
	stats := tenStats()

	if out := StatPanel(th, stats, 80, 20); !strings.Contains(out, "╭") {
		t.Errorf("roomy panel should be boxed:\n%s", out)
	}

	flat := StatPanel(th, stats, 80, 6)
	if strings.Contains(flat, "╭") || Lines(flat) > 6 {
		t.Errorf("panel in 6 lines should be unboxed and fit:\n%s", flat)
	}
	for _, s := range stats {
		if !strings.Contains(flat, strings.ToUpper(s.Label)) {
			t.Errorf("unboxed panel lost %q:\n%s", s.Label, flat)
		}
	}

	tight := StatPanel(th, stats, 80, 2)
	if Lines(tight) != 2 {
		t.Errorf("panel in 2 lines drew %d:\n%s", Lines(tight), tight)
	}
	if !strings.Contains(tight, "STAT 0") {
		t.Errorf("a tight panel keeps the first stats:\n%s", tight)
	}
}

func TestSideBySideStartsEachColumnOnOneEdge(t *testing.T) {
	out := SideBySide(2, []int{6, 10}, "a\nlonger line\nb", "x\ny")
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d:\n%s", len(lines), out)
	}
	for i, want := range []string{"x", "y"} {
		at := strings.Index(lines[i], want)
		if got := lipgloss.Width(lines[i][:max(at, 0)]); at < 0 || got != 8 {
			t.Errorf("line %d: %q starts at %d, want 8:\n%s", i, want, got, out)
		}
	}
	if w := lipgloss.Width(lines[1]); w > 18 {
		t.Errorf("an overlong left line pushed the right column: %q", lines[1])
	}
}

func TestWindowTabsKeepsTheSelectionInView(t *testing.T) {
	th := styles.Plain()
	var tabs []string
	for y := 2000; y <= 2026; y++ {
		tabs = append(tabs, strconv.Itoa(y))
	}
	for _, sel := range []int{0, 13, 26} {
		out := WindowTabs(th, tabs, sel, 30)
		if !strings.Contains(out, tabs[sel]) {
			t.Errorf("selection %s scrolled out of view: %q", tabs[sel], out)
		}
		if w := lipgloss.Width(out); w > 30 {
			t.Errorf("bar is %d wide, want at most 30: %q", w, out)
		}
	}
	if out := WindowTabs(th, tabs, 26, 30); !strings.HasPrefix(out, "‹") {
		t.Errorf("seasons cut off the left should be marked: %q", out)
	}
}
