package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Accent is footui's highlight colour on screens no club owns.
type Accent struct {
	// empty means lime
	hex string
	// draw in the terminal's ANSI palette
	terminal bool
}

// Terminal reports whether footui follows the terminal's palette.
func (a Accent) Terminal() bool { return a.terminal }

func (a Accent) String() string {
	switch {
	case a.terminal:
		return "terminal"
	case a.hex != "":
		return a.hex
	}
	return "lime"
}

// ParseAccent reads an accent setting: "lime" (or nothing), "terminal",
// or a colour as #rrggbb.
func ParseAccent(s string) (Accent, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "lime":
		return Accent{}, nil
	case "terminal":
		return Accent{terminal: true}, nil
	}
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	if len(s) != 7 {
		return Accent{}, fmt.Errorf("unknown accent %q (want lime, terminal, or a colour like #ff8800)", s)
	}
	c, ok := parseHex(s)
	if !ok {
		return Accent{}, fmt.Errorf("unknown accent %q (want lime, terminal, or a colour like #ff8800)", s)
	}
	return Accent{hex: c.Hex()}, nil
}

var accent Accent

// UseAccent sets footui's accent for every theme built afterwards.
func UseAccent(a Accent) { accent = a }

// applyAccent puts the accent in use on footui's own palette.
func applyAccent(p Palette) Palette {
	switch {
	case accent.terminal:
		return terminalPalette()
	case accent.hex != "":
		c := mustHex(accent.hex)
		p.Accent = adaptiveForeground(c, minContrast)
		p.OnAccent = lipgloss.AdaptiveColor{
			Light: readableHex(adjustForBackground(c, mustHex(lightBackground), minContrast)),
			Dark:  readableHex(adjustForBackground(c, mustHex(darkBackground), minContrast)),
		}
	}
	return p
}

// terminalPalette is footui's palette in the terminal's ANSI colours.
func terminalPalette() Palette {
	return Palette{
		Accent:    lipgloss.ANSIColor(10), // bright green
		OnAccent:  lipgloss.ANSIColor(0),
		Text:      lipgloss.NoColor{},
		Muted:     lipgloss.ANSIColor(7),
		Faint:     lipgloss.ANSIColor(8),
		Border:    lipgloss.ANSIColor(8),
		Positive:  lipgloss.ANSIColor(2),
		Negative:  lipgloss.ANSIColor(1),
		Draw:      lipgloss.ANSIColor(3),
		Info:      lipgloss.ANSIColor(6),
		Below:     lipgloss.ANSIColor(11),
		Highlight: lipgloss.ANSIColor(8),
	}
}

func readableHex(c colorful.Color) string {
	return string(readableTextOn(c).(lipgloss.Color))
}

// strongAccentHex is the pitch's accent. The pitch is always footui's
// green, so a terminal accent still gets lime.
func strongAccentHex() string {
	if accent.hex != "" {
		return accent.hex
	}
	return baseLime
}
