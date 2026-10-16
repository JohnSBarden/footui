package components

import "strings"

// digitFont draws 0–9 three rows tall in half blocks, for the one number on
// a screen worth reading from across the room.
var digitFont = map[rune][3]string{
	'0': {"█▀█", "█ █", "▀▀▀"}, '1': {"▀█ ", " █ ", "▀▀▀"},
	'2': {"▀▀█", "█▀▀", "▀▀▀"}, '3': {"▀▀█", " ▀█", "▀▀▀"},
	'4': {"█ █", "▀▀█", "  ▀"}, '5': {"█▀▀", "▀▀█", "▀▀▀"},
	'6': {"█▀▀", "█▀█", "▀▀▀"}, '7': {"▀▀█", "  █", "  ▀"},
	'8': {"█▀█", "█▀█", "▀▀▀"}, '9': {"█▀█", "▀▀█", "▀▀▀"},
}

// BlockDigits renders s's digits in digitFont, one space between them. Any
// non-digit makes the whole number unrenderable: an empty result.
func BlockDigits(s string) [3]string {
	var rows [3][]string
	for _, r := range s {
		g, ok := digitFont[r]
		if !ok {
			return [3]string{}
		}
		for i := range rows {
			rows[i] = append(rows[i], g[i])
		}
	}
	return [3]string{strings.Join(rows[0], " "), strings.Join(rows[1], " "), strings.Join(rows[2], " ")}
}
