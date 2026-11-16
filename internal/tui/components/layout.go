package components

// FitColumns appends optional column groups, in order, while they fit.
// A narrow terminal gives up detail, never identity: the name column keeps
// nameWidth (squeezed names become indistinguishable), and groups drop
// whole because some stats only mean something in pairs (xG with xGA).
func FitColumns(width int, base []Column, nameWidth int, groups [][]Column) []Column {
	// Each column costs its width plus a separating space; slop is the indent.
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
			// Stop at the first misfit so a narrower later group can't
			// sneak in after a wider one was cut.
			break
		}
		used += cost
		out = append(out, group...)
	}

	return out
}

// TrimRow drops cells whose columns didn't fit.
func TrimRow(row []Cell, columns int) []Cell {
	if len(row) > columns {
		return row[:columns]
	}
	return row
}

// DropBlankColumns removes the group columns that are "—" on every row:
// a source that has no xG shouldn't cost a column of dashes. rows hold the
// base cells first, then the groups' in order, as FitColumns lays them out.
// A group left with no columns goes too. With no rows nothing is dropped.
func DropBlankColumns(base []Column, groups [][]Column, rows [][]Cell) ([][]Column, [][]Cell) {
	if len(rows) == 0 {
		return groups, rows
	}
	keep := make([]int, 0, len(base))
	for i := range base {
		keep = append(keep, i)
	}
	var kept [][]Column
	i := len(base)
	for _, g := range groups {
		var cols []Column
		for _, c := range g {
			if !blankColumn(rows, i) {
				cols = append(cols, c)
				keep = append(keep, i)
			}
			i++
		}
		if len(cols) > 0 {
			kept = append(kept, cols)
		}
	}
	out := make([][]Cell, len(rows))
	for r, row := range rows {
		out[r] = make([]Cell, 0, len(keep))
		for _, k := range keep {
			if k < len(row) {
				out[r] = append(out[r], row[k])
			}
		}
	}
	return kept, out
}

func blankColumn(rows [][]Cell, i int) bool {
	for _, row := range rows {
		if i < len(row) && row[i].Text != "—" {
			return false
		}
	}
	return true
}
