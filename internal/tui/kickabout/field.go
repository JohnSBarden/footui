// Package kickabout turns whatever screen is showing into a pitch: the
// screen's own text stays where it was as scenery, a few of its words come
// alive as defenders, and you dribble round them into a goal that won't
// keep still.
package kickabout

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Field is a screen frame as a grid of cells. Its text is only scenery:
// the ball and the player go straight over it, and only the edges stop
// them.
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

// blocked reports whether (x, y) is off the field or a cell block reports.
func (f Field) blocked(x, y int, block func(x, y int) bool) bool {
	return !f.inside(x, y) || block != nil && block(x, y)
}

// Rune is the character at (x, y), a space off the field.
func (f Field) Rune(x, y int) rune {
	if !f.inside(x, y) {
		return ' '
	}
	return f.cells[y][x]
}

// Word is a run of non-space cells starting at (X, Y).
type Word struct {
	X, Y int
	Text string
}

// Words are the field's runs of non-space cells that read as words: 2 to
// 12 runes with at least one letter, so rules, borders and lone numbers
// are left out.
func (f Field) Words() []Word {
	var ws []Word
	for y, row := range f.cells {
		for x := 0; x < f.W; {
			if row[x] == ' ' {
				x++
				continue
			}
			end := x
			for end < f.W && row[end] != ' ' {
				end++
			}
			if w := row[x:end]; len(w) >= 2 && len(w) <= 12 && hasLetter(w) {
				ws = append(ws, Word{X: x, Y: y, Text: string(w)})
			}
			x = end
		}
	}
	return ws
}

func hasLetter(rs []rune) bool {
	for _, r := range rs {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// Lift blanks w's cells: the word has walked off the screen to play.
func (f Field) Lift(w Word) {
	for i := range []rune(w.Text) {
		if f.inside(w.X+i, w.Y) {
			f.cells[w.Y][w.X+i] = ' '
		}
	}
}
