package components

import "testing"

var (
	nameCol  = 24
	baseCols = []Column{
		{Title: "#", Width: 3, Right: true},
		{Title: "Pts", Width: 4, Right: true},
	}
	groups = [][]Column{
		{{Title: "xG", Width: 6, Right: true}, {Title: "xGA", Width: 6, Right: true}},
		{{Title: "xGD", Width: 6, Right: true}},
	}
)

func titles(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Title
	}
	return out
}

func has(cols []Column, title string) bool {
	for _, c := range cols {
		if c.Title == title {
			return true
		}
	}
	return false
}

func TestWideTerminalGetsEveryGroup(t *testing.T) {
	got := FitColumns(200, baseCols, nameCol, groups)
	for _, want := range []string{"xG", "xGA", "xGD"} {
		if !has(got, want) {
			t.Errorf("width 200 dropped %q: %v", want, titles(got))
		}
	}
}

func TestNarrowTerminalKeepsOnlyTheBaseColumns(t *testing.T) {
	got := FitColumns(40, baseCols, nameCol, groups)
	if len(got) != len(baseCols) {
		t.Errorf("width 40 should keep only base columns, got %v", titles(got))
	}
}

// Dropping half a pair would leave a column nobody can read a season from.
func TestGroupsAreDroppedWhole(t *testing.T) {
	for width := 30; width < 120; width++ {
		got := FitColumns(width, baseCols, nameCol, groups)
		if has(got, "xG") != has(got, "xGA") {
			t.Fatalf("width %d split the xG/xGA pair: %v", width, titles(got))
		}
	}
}

// A later, narrower group must not sneak in after a wider one was cut, or
// the drop order becomes unpredictable across widths.
func TestDropOrderIsMonotonic(t *testing.T) {
	for width := 30; width < 120; width++ {
		got := FitColumns(width, baseCols, nameCol, groups)
		if has(got, "xGD") && !has(got, "xG") {
			t.Fatalf("width %d kept a later group after dropping an earlier one: %v", width, titles(got))
		}
	}
}

// More width never means fewer columns.
func TestColumnCountNeverDecreasesWithWidth(t *testing.T) {
	prev := 0
	for width := 30; width < 200; width++ {
		got := len(FitColumns(width, baseCols, nameCol, groups))
		if got < prev {
			t.Fatalf("width %d rendered %d columns after %d at a narrower width", width, got, prev)
		}
		prev = got
	}
}

func TestTrimRowMatchesTheRenderedColumns(t *testing.T) {
	row := []Cell{Text("a"), Text("b"), Text("c"), Text("d")}
	if got := TrimRow(row, 2); len(got) != 2 {
		t.Errorf("TrimRow(_, 2) kept %d cells", len(got))
	}
	if got := TrimRow(row, 10); len(got) != 4 {
		t.Errorf("TrimRow should not pad, got %d cells", len(got))
	}
}
