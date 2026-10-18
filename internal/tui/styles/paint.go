package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// stripeWidth is how many columns each mowing stripe spans, the same cut
// as the kickabout's pitch.
const stripeWidth = 6

// Paint fills a rendered frame with the theme's background, edge to edge,
// cut into mowing stripes when the theme has a second shade.
//
// Giving the outermost style a background isn't enough: every styled span
// inside ends with a full reset, and from there to the next span the
// terminal's own background shows through. And no single background can
// draw stripes. So each line is walked cell by cell: the stripe for the
// column is put back after every reset and switched at every stripe
// boundary, except inside a span that brought its own background (a
// keycap, the selected row), which is left exactly as it was. Lines are
// padded to the full width and blank lines fill out the height.
//
// A theme with no background, or a terminal with no color, gets the frame
// back untouched.
func Paint(frame string, th Theme, width, height int) string {
	if th.Background == nil {
		return frame
	}
	profile := lipgloss.ColorProfile()
	if profile == termenv.Ascii {
		return frame
	}
	bgs, ok := stripeSequences(profile, th)
	if !ok {
		return frame
	}
	fg := ""
	if c := profile.Color(colorHex(th.Palette.Text)); c != nil {
		fg = termenv.CSI + c.Sequence(false) + "m"
	}

	lines := strings.Split(frame, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}

	var b strings.Builder
	for i, line := range lines {
		paintLine(&b, line, width, fg, bgs)
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// stripeSequences are the escape sequences setting each stripe's
// background; a flat theme has the same one twice.
func stripeSequences(profile termenv.Profile, th Theme) ([2]string, bool) {
	var seqs [2]string
	shades := [2]lipgloss.TerminalColor{th.Background, th.Stripe}
	if th.Stripe == nil {
		shades[1] = th.Background
	}
	for i, shade := range shades {
		c := profile.Color(colorHex(shade))
		if c == nil {
			return seqs, false
		}
		seqs[i] = termenv.CSI + c.Sequence(true) + "m"
	}
	return seqs, true
}

// paintLine writes one line of the frame with the stripes laid behind it.
func paintLine(b *strings.Builder, line string, width int, fg string, bgs [2]string) {
	col, shown, own := 0, -1, false
	// ensure puts the stripe for col down, unless a span's own background
	// is in force or that stripe is already showing.
	ensure := func() {
		if own {
			return
		}
		if s := (col / stripeWidth) % 2; s != shown {
			b.WriteString(bgs[s])
			shown = s
		}
	}

	b.WriteString(fg)
	var state byte
	for len(line) > 0 {
		seq, w, n, newState := ansi.DecodeSequence(line, state, nil)
		state = newState
		line = line[n:]
		if w == 0 {
			b.WriteString(seq)
			if params, ok := sgrParams(seq); ok {
				var reset bool
				own, reset = applySGR(own, params)
				if reset {
					b.WriteString(fg)
				}
				// Whatever the span did to the background, the stripe
				// must be re-established before the next cell.
				shown = -1
			}
			continue
		}
		ensure()
		b.WriteString(seq)
		col += w
	}
	for ; col < width; col++ {
		ensure()
		b.WriteByte(' ')
	}
	b.WriteString(reset)
}

// reset is the sequence Lip Gloss ends every styled span with.
const reset = "\x1b[0m"

// sgrParams is the parameter list of a Select Graphic Rendition sequence
// (ESC [ ... m), or false for any other sequence.
func sgrParams(seq string) ([]string, bool) {
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		return nil, false
	}
	body := seq[2 : len(seq)-1]
	if strings.ContainsAny(body, "<=>?") {
		return nil, false
	}
	return strings.FieldsFunc(body, func(r rune) bool { return r == ';' || r == ':' }), true
}

// applySGR folds an SGR's parameters into whether a span's own background
// is in force, and reports whether they included a full reset (which also
// clears the text color the frame is painted with).
func applySGR(own bool, params []string) (bool, bool) {
	if len(params) == 0 {
		return false, true
	}
	reset := false
	for i := 0; i < len(params); i++ {
		switch p := params[i]; {
		case p == "0":
			own, reset = false, true
		case p == "49":
			own = false
		case p == "7":
			// Reverse video turns the text color into a background.
			own = true
		case p == "27":
			own = false
		case p == "48":
			own = true
			i += extendedColorArgs(params[i+1:])
		case p == "38" || p == "58":
			i += extendedColorArgs(params[i+1:])
		case len(p) == 2 && p[0] == '4' && p[1] >= '0' && p[1] <= '7',
			len(p) == 3 && p[:2] == "10" && p[2] >= '0' && p[2] <= '7':
			own = true
		}
	}
	return own, reset
}

// extendedColorArgs is how many parameters follow a 38/48/58: "5;n" for a
// palette index, "2;r;g;b" for true color.
func extendedColorArgs(rest []string) int {
	if len(rest) == 0 {
		return 0
	}
	switch rest[0] {
	case "5":
		return min(2, len(rest))
	case "2":
		return min(4, len(rest))
	}
	return 0
}

// colorHex extracts a concrete color string. Strong themes only ever use
// concrete colors; an adaptive one yields "" and is skipped.
func colorHex(c lipgloss.TerminalColor) string {
	if v, ok := c.(lipgloss.Color); ok {
		return string(v)
	}
	return ""
}
