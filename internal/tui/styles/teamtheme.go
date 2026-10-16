package styles

import (
	"math"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// A club's brand colors are chosen for shirts and crests, not for text on a
// terminal. Navy on black and yellow on white are both unreadable, and a
// terminal's background is not knowable in advance — it may be light, dark,
// or change while the program is running. So brand colors are never used
// raw: they are adjusted per terminal background, and the two adjusted
// variants are handed to Lip Gloss as an AdaptiveColor.
const (
	// minContrast is the WCAG AA ratio for normal-size text. Brand colors
	// are lightened or darkened until they clear it.
	minContrast = 4.5
	// minLargeContrast is WCAG AA for large text, applied to the header
	// bar where the club color is a background behind bold text. Holding
	// the header to the full 4.5 would wash out nearly every club's
	// primary into something unrecognizable.
	minLargeContrast = 3.0

	// The backgrounds adjustment targets. Terminals are rarely pure white
	// or pure black, so these are the realistic extremes.
	lightBackground = "#ffffff"
	darkBackground  = "#0b0b0b"
)

// TeamPalette derives a readable palette from a club's brand colors.
// secondary and tertiary may be empty; the primary alone is enough.
func TeamPalette(primaryHex, secondaryHex string) Palette {
	base := DefaultPalette()

	primary, ok := parseHex(primaryHex)
	if !ok {
		// An unparseable or missing brand color falls back to the neutral
		// palette rather than rendering a club in an arbitrary color.
		return base
	}

	base.Accent = adaptiveForeground(primary, minContrast)
	base.OnAccent = readableTextOn(primary)
	base.Highlight = selectionBackground(primary)

	if secondary, ok := parseHex(secondaryHex); ok {
		// The secondary color is used where a second brand accent helps
		// (panel borders); it gets the same readability treatment.
		base.Border = adaptiveForeground(secondary, minLargeContrast)
	}

	return base
}

// ClubForeground is a club colour made readable as a mark (a dot, a stripe)
// on any terminal background, or false when hex isn't a colour.
func ClubForeground(hex string) (lipgloss.TerminalColor, bool) {
	c, ok := parseHex(hex)
	if !ok {
		return nil, false
	}
	return adaptiveForeground(c, minLargeContrast), true
}

// TeamTheme builds a full Theme from a club's brand colors.
func TeamTheme(primaryHex, secondaryHex string) Theme {
	return New(TeamPalette(primaryHex, secondaryHex))
}

// TeamThemeFor builds a club's theme in the given mode.
func TeamThemeFor(primaryHex, secondaryHex string, mode Mode) Theme {
	if mode == Strong {
		if t, ok := StrongTeamTheme(primaryHex, secondaryHex); ok {
			return t
		}
	}
	return TeamTheme(primaryHex, secondaryHex)
}

// StrongTeamTheme builds a theme whose background is the club's primary
// color. Unlike the weak theme the background is known exactly, so every
// color is solved against it once rather than adapted per terminal: text
// is black or white, whichever contrasts more; the secondary color becomes
// the accent if it can be made readable on the primary; and the selected
// row and title bar are drawn inverse, the strongest contrast available.
func StrongTeamTheme(primaryHex, secondaryHex string) (Theme, bool) {
	bg, ok := parseHex(primaryHex)
	if !ok {
		return Theme{}, false
	}
	text := mustHex("#ffffff")
	if contrastRatio(mustHex("#000000"), bg) > contrastRatio(text, bg) {
		text = mustHex("#000000")
	}

	accent := text
	if secondary, ok := parseHex(secondaryHex); ok {
		if c := solveOn(secondary, bg, minContrast); contrastRatio(c, bg) >= minContrast {
			accent = c
		}
	}

	hex := func(c colorful.Color) lipgloss.Color { return lipgloss.Color(c.Hex()) }
	p := Palette{
		Accent:    hex(accent),
		OnAccent:  readableTextOn(accent),
		Text:      hex(text),
		Muted:     hex(muted(text, bg)),
		Border:    hex(accent),
		Positive:  hex(solveOn(mustHex("#2ec27e"), bg, minContrast)),
		Negative:  hex(solveOn(mustHex("#e01b24"), bg, minContrast)),
		Draw:      hex(solveOn(mustHex("#F5C04A"), bg, minContrast)),
		Info:      hex(solveOn(mustHex("#4CC9F0"), bg, minContrast)),
		Below:     hex(solveOn(mustHex("#FF9F43"), bg, minContrast)),
		Faint:     hex(muted(text, bg)),
		Highlight: hex(text),
	}

	t := New(p)
	t.Background = hex(bg)
	t.Title = t.Title.Foreground(hex(bg)).Background(hex(text))
	t.TableSelected = lipgloss.NewStyle().Foreground(hex(bg)).Background(hex(text)).Bold(true)
	return t, true
}

// solveOn adjusts c's lightness until it reads against bg, trying both
// directions — on a mid-tone background either may be the one that works
// — and keeping whichever gets closer.
func solveOn(c, bg colorful.Color, threshold float64) colorful.Color {
	if contrastRatio(c, bg) >= threshold {
		return c
	}
	h, s, l := c.Hsl()
	best, bestRatio := c, contrastRatio(c, bg)
	for _, step := range []float64{0.02, -0.02} {
		for v := l + step; v >= 0 && v <= 1; v += step {
			candidate := quantize(colorful.Hsl(h, s, v))
			r := contrastRatio(candidate, bg)
			if r >= threshold {
				// The first passing step in this direction is the least
				// change; prefer it over a bigger change the other way.
				if bestRatio < threshold || colorDistance(candidate, c) < colorDistance(best, c) {
					best, bestRatio = candidate, r
				}
				break
			}
			if bestRatio < threshold && r > bestRatio {
				best, bestRatio = candidate, r
			}
		}
	}
	return best
}

// quantize rounds a color to what it will actually be rendered as: an
// in-gamut, 8-bit-per-channel hex value. Contrast is checked on this,
// not on the float color, or a blend that clears the threshold
// in-memory can miss it once written out.
func quantize(c colorful.Color) colorful.Color {
	return mustHex(c.Clamped().Hex())
}

func colorDistance(a, b colorful.Color) float64 { return a.DistanceLab(b) }

// muted blends text toward the background as far as it can while staying
// readable, giving secondary text that recedes without disappearing.
func muted(text, bg colorful.Color) colorful.Color {
	best := text
	for f := 0.05; f < 1; f += 0.05 {
		c := quantize(text.BlendLab(bg, f))
		if contrastRatio(c, bg) < minContrast {
			break
		}
		best = c
	}
	return best
}

// parseHex parses "#rrggbb" (or "rrggbb"), reporting false for anything
// malformed.
func parseHex(hex string) (colorful.Color, bool) {
	if hex == "" {
		return colorful.Color{}, false
	}
	if hex[0] != '#' {
		hex = "#" + hex
	}
	c, err := colorful.Hex(hex)
	if err != nil {
		return colorful.Color{}, false
	}
	return c, true
}

// adaptiveForeground returns the brand color adjusted separately for light
// and dark terminals, each lightened or darkened only as far as needed to
// clear the contrast threshold. Adjusting in HSL keeps the hue — the part
// that makes the color recognizably the club's — while moving only
// lightness.
func adaptiveForeground(c colorful.Color, threshold float64) lipgloss.TerminalColor {
	return lipgloss.AdaptiveColor{
		Light: adjustForBackground(c, mustHex(lightBackground), threshold).Hex(),
		Dark:  adjustForBackground(c, mustHex(darkBackground), threshold).Hex(),
	}
}

// adjustForBackground walks a color's lightness toward the opposite end
// from bg until it meets the contrast threshold, giving up at the extreme.
func adjustForBackground(c, bg colorful.Color, threshold float64) colorful.Color {
	if contrastRatio(c, bg) >= threshold {
		return c
	}

	h, s, l := c.Hsl()
	// Move away from the background: darken on a light background, lighten
	// on a dark one.
	step := 0.02
	if relativeLuminance(bg) > 0.5 {
		step = -step
	}

	best := c
	for i := 0; i < 50; i++ {
		l += step
		if l < 0 || l > 1 {
			break
		}
		candidate := colorful.Hsl(h, s, l)
		best = candidate
		if contrastRatio(candidate, bg) >= threshold {
			return candidate
		}
	}
	// Nothing in this hue cleared the threshold — return the most extreme
	// version tried, which is still the closest to readable.
	return best
}

// readableTextOn picks black or white for text drawn on top of a brand
// color, whichever contrasts more. This is what keeps the header bar
// legible for both a near-black club (Newcastle) and a near-white one
// (Real Madrid).
func readableTextOn(bg colorful.Color) lipgloss.TerminalColor {
	black, white := mustHex("#000000"), mustHex("#ffffff")
	if contrastRatio(white, bg) >= contrastRatio(black, bg) {
		return lipgloss.Color("#ffffff")
	}
	return lipgloss.Color("#000000")
}

// selectionBackground derives the selected-row background from the brand
// color: heavily desaturated and pushed toward the terminal background, so
// it tints the row without competing with the text on it.
func selectionBackground(c colorful.Color) lipgloss.TerminalColor {
	h, s, _ := c.Hsl()
	return lipgloss.AdaptiveColor{
		Light: colorful.Hsl(h, clamp(s*0.5), 0.86).Hex(),
		Dark:  colorful.Hsl(h, clamp(s*0.5), 0.22).Hex(),
	}
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// relativeLuminance is the WCAG 2.x relative luminance of a color.
func relativeLuminance(c colorful.Color) float64 {
	channel := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		// The WCAG gamma expansion.
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

// contrastRatio is the WCAG contrast ratio between two colors, from 1
// (identical) to 21 (black against white).
func contrastRatio(a, b colorful.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func mustHex(hex string) colorful.Color {
	c, err := colorful.Hex(hex)
	if err != nil {
		// Only ever called with the literals above, so this cannot fire
		// at runtime; panicking beats silently returning black.
		panic("styles: bad built-in color " + hex)
	}
	return c
}
