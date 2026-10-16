package components

import (
	"math"
	"strings"
)

// eighths are the partial blocks a bar ends in, so a bar's length is exact
// to an eighth of a cell rather than rounded to whole cells.
var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// Bar draws value on a scale of max across width cells. A non-positive
// value or scale draws nothing; a value past max is drawn full.
func Bar(value, max float64, width int) string {
	if value <= 0 || max <= 0 || width <= 0 {
		return ""
	}
	cells := math.Min(value/max, 1) * float64(width)
	full := int(cells)
	part := int(math.Round((cells - float64(full)) * 8))
	if part == 8 {
		full, part = full+1, 0
	}
	return strings.Repeat("█", full) + eighths[part]
}

// Diverging splits a signed value into a bar left of zero (neg, which the
// caller right-aligns) or right of it (pos), each up to half cells.
func Diverging(value, maxAbs float64, half int) (neg, pos string) {
	if value < 0 {
		return Bar(-value, maxAbs, half), ""
	}
	return "", Bar(value, maxAbs, half)
}
