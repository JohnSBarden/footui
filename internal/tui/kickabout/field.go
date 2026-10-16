// Package kickabout turns whatever screen is showing into a pitch: the
// screen's own text stays where it was, as obstacles, and you play a ball
// around it into a goal that won't keep still.
package kickabout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Field is a screen frame as a grid of cells; any non-space cell is solid.
type Field struct {
	W, H  int
	cells [][]rune
}

// NewField snapshots frame, cut or padded to width×height.
func NewField(frame string, width, height int) Field {
	lines := strings.Split(ansi.Strip(frame), "\n")
	f := Field{W: width, H: height, cells: make([][]rune, height)}
	for y := range f.cells {
		row := []rune(strings.Repeat(" ", width))
		if y < len(lines) {
			x := 0
			for _, r := range lines[y] {
				if x >= width {
					break
				}
				row[x] = r
				x++
			}
		}
		f.cells[y] = row
	}
	return f
}

func (f Field) inside(x, y int) bool { return x >= 0 && y >= 0 && x < f.W && y < f.H }

// Solid reports whether (x, y) is text or off the field.
func (f Field) Solid(x, y int) bool {
	return !f.inside(x, y) || f.cells[y][x] != ' '
}

// Rune is the character at (x, y), a space off the field.
func (f Field) Rune(x, y int) rune {
	if !f.inside(x, y) {
		return ' '
	}
	return f.cells[y][x]
}

// WordAt is the run of non-space cells through (x, y): the word the ball
// just hit, or "" for an empty cell or one off the field.
func (f Field) WordAt(x, y int) string {
	if !f.inside(x, y) || f.cells[y][x] == ' ' {
		return ""
	}
	row := f.cells[y]
	lo, hi := x, x
	for lo > 0 && row[lo-1] != ' ' {
		lo--
	}
	for hi < f.W-1 && row[hi+1] != ' ' {
		hi++
	}
	return string(row[lo : hi+1])
}
