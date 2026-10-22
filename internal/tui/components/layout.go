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
