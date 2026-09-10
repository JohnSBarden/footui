package components

// FitColumns decides which optional column groups fit at the current
// terminal width.
//
// The rule it encodes: a narrow terminal gives up detail, never identity.
// Squeezing a name column turns "Manchester United" and "Manchester City"
// into two identical truncated rows, which makes a table actively wrong
// rather than merely less detailed — so the name column keeps its width
// and derived statistics are dropped instead, cheapest first.
//
// Groups are dropped whole. Some statistics only mean something in pairs:
// attacking output without what the team conceded is a column nobody can
// read a season from, so xG and xGA travel together.
//
// base are the columns that always render. nameWidth is what the name
// column needs. groups are tried in order and appended while they fit.
func FitColumns(width int, base []Column, nameWidth int, groups [][]Column) []Column {
	// Each column costs its own width plus the single space separating it
	// from the next, and a little slop for the leading indent.
	const slop = 2
	used := slop + nameWidth + 1
	for _, c := range base {
		used += c.Width + 1
	}

	out := make([]Column, 0, len(base)+len(groups)*2)
	out = append(out, base...)

	for _, group := range groups {
		cost := 0
		for _, c := range group {
			cost += c.Width + 1
		}
		if used+cost > width {
			// Later groups are progressively wider or equally optional;
			// stopping here keeps the drop order predictable rather than
			// letting a narrow column sneak in after a wide one was cut.
			break
		}
		used += cost
		out = append(out, group...)
	}

	return out
}

// TrimRow shortens a row to the number of columns actually being
// rendered. Rows are built with every cell so the builder stays simple;
// this drops the ones whose columns didn't fit.
func TrimRow(row []Cell, columns int) []Cell {
	if len(row) > columns {
		return row[:columns]
	}
	return row
}
