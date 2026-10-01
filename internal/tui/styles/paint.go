package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// reset is the sequence Lip Gloss ends every styled span with. It clears
// the background along with everything else, which is the whole problem
// Paint solves.
const reset = "\x1b[0m"

// Paint fills a rendered frame with the theme's background, edge to edge.
//
// Giving the outermost style a background isn't enough: every styled span
// inside ends with a full reset, and from there to the next span the
// terminal's own background shows through, leaving the frame striped. So
// the base colors are re-applied after every reset, each line is padded
// to the full width, and blank lines fill out the height.
//
// A theme with no background, or a terminal with no color, gets the frame
// back untouched.
func Paint(frame string, th Theme, width, height int) string {
	if th.Background == nil {
		return frame
	}
	base := baseSequence(th)
	if base == "" {
		return frame
	}

	lines := strings.Split(frame, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}

	var b strings.Builder
	for i, line := range lines {
		pad := width - ansi.StringWidth(line)
		b.WriteString(base)
		b.WriteString(strings.ReplaceAll(line, reset, reset+base))
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(reset)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// baseSequence is the escape sequence setting the theme's background and
// default text color, or "" when the terminal can't show them.
func baseSequence(th Theme) string {
	profile := lipgloss.ColorProfile()
	if profile == termenv.Ascii {
		return ""
	}
	bg := profile.Color(colorHex(th.Background))
	if bg == nil {
		return ""
	}
	seq := bg.Sequence(true)
	if fg := profile.Color(colorHex(th.Palette.Text)); fg != nil {
		seq = fg.Sequence(false) + ";" + seq
	}
	return termenv.CSI + seq + "m"
}

// colorHex extracts a concrete color string. Strong themes only ever use
// concrete colors; an adaptive one yields "" and is skipped.
func colorHex(c lipgloss.TerminalColor) string {
	if v, ok := c.(lipgloss.Color); ok {
		return string(v)
	}
	return ""
}
