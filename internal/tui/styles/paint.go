package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// stripeWidth is columns per mowing stripe, matching the kickabout pitch.
const stripeWidth = 6

// Paint fills a rendered frame with the theme's background, edge to edge,
// cut into mowing stripes when the theme has a second shade.
//
// An outer background isn't enough: every inner span ends with a full
// reset, letting the terminal's background through. So the stripe is
// re-applied after each reset, except inside spans with their own
// background.
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

// Shade lays bg behind one rendered line, as Paint does for a frame.
func Shade(line string, bg lipgloss.TerminalColor) string {
	if bg == nil {
		return line
	}
	seq := backgroundSequence(bg)
	if seq == "" {
		return line
	}
	var b strings.Builder
	b.WriteString(seq)
	var state byte
	for len(line) > 0 {
		s, _, n, newState := ansi.DecodeSequence(line, state, nil)
		state = newState
		line = line[n:]
		b.WriteString(s)
		if params, ok := sgrParams(s); ok {
			if own, _ := applySGR(true, params); !own {
				b.WriteString(seq)
			}
		}
	}
	b.WriteString(reset)
	return b.String()
}

// backgroundSequence is the SGR setting bg in the terminal's profile,
// adaptive colours included; empty when the terminal has no colour.
func backgroundSequence(bg lipgloss.TerminalColor) string {
	out := lipgloss.NewStyle().Background(bg).Render("x")
	seq, _, ok := strings.Cut(out, "x")
	if !ok {
		return ""
	}
	return seq
}

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

func paintLine(b *strings.Builder, line string, width int, fg string, bgs [2]string) {
	col, shown, own := 0, -1, false
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
				// The stripe must be re-established before the next cell.
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

// sgrParams is the parameter list of an SGR (ESC [ ... m) sequence.
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

// applySGR reports whether a span's own background is in force, and
// whether the params included a full reset (which also clears the fg).
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
			// Reverse video turns the text colour into a background.
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

// extendedColorArgs is how many params follow a 38/48/58: "5;n" or
// "2;r;g;b".
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

// colorHex extracts a concrete colour; an adaptive one yields "".
func colorHex(c lipgloss.TerminalColor) string {
	if v, ok := c.(lipgloss.Color); ok {
		return string(v)
	}
	return ""
}
