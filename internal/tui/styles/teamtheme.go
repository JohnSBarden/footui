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

// HeroStyles are a club screen's header band and the rule beneath it. The
// band is the club's primary with black or white text, whichever reads;
// in strong mode the whole screen already is the primary, so the band
// inverts. A club without a usable colour gets footui's accent.
func HeroStyles(th Theme, primaryHex, secondaryHex string, mode Mode) (fill, rule lipgloss.Style) {
	if th.NoColor {
		return lipgloss.NewStyle(), lipgloss.NewStyle()
	}
	primary, ok := parseHex(primaryHex)
	if !ok {
		fill = lipgloss.NewStyle().Foreground(th.Palette.OnAccent).Background(th.Palette.Accent)
		return fill, lipgloss.NewStyle().Foreground(th.Palette.Accent)
	}
	var bg, fg lipgloss.TerminalColor = lipgloss.Color(primary.Hex()), readableTextOn(primary)
	if mode == Strong {
		bg, fg = th.Palette.Text, lipgloss.Color(primary.Hex())
	}
	fill = lipgloss.NewStyle().Foreground(fg).Background(bg)
	rule = lipgloss.NewStyle().Foreground(bg)
	if s, ok := parseHex(secondaryHex); ok {
		rule = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Hex()))
	}
	return fill, rule
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
// color, cut into mowing stripes: the primary itself alternating with a
// shade of it, like a pitch. Unlike the weak theme the background is known
// exactly, so every color is solved against both shades once rather than
// adapted per terminal: text is black or white, whichever contrasts more;
// the secondary color becomes the accent if it can be made readable on
// the primary; and the selected row and title bar are drawn inverse, the
// strongest contrast available.
func StrongTeamTheme(primaryHex, secondaryHex string) (Theme, bool) {
	primary, ok := parseHex(primaryHex)
	if !ok {
		return Theme{}, false
	}
	return strongTheme(primary, primary, mowingShade(primary), secondaryHex), true
}

// stripeSpread is how far apart in lightness the two mowing shades sit:
// about the gap between the kickabout's two greens, enough to see the cut
// without the stripes competing with the text on them.
const stripeSpread = 0.044

// mowingShade is the second stripe for a primary: the same hue moved away
// from whichever of black or white text reads on it — darker under white
// text, lighter under black — so the cut never costs the text contrast.
// A primary already at that extreme goes the other way, where there's
// contrast to spare.
func mowingShade(c colorful.Color) colorful.Color {
	h, s, l := c.Hsl()
	darker := contrastRatio(mustHex("#ffffff"), c) >= contrastRatio(mustHex("#000000"), c)
	if darker && l-stripeSpread < 0 || !darker && l+stripeSpread > 1 {
		darker = !darker
	}
	if darker {
		return quantize(colorful.Hsl(h, s, l-stripeSpread))
	}
	return quantize(colorful.Hsl(h, s, l+stripeSpread))
}

// strongTheme builds a strong theme on stripes a and b. primary is the
// color drawn on inverse elements (the title bar, keys, the selected row).
func strongTheme(primary, a, b colorful.Color, secondaryHex string) Theme {
	bgs := []colorful.Color{a, b}
	text := mustHex("#ffffff")
	if worstContrast(mustHex("#000000"), bgs) > worstContrast(text, bgs) {
		text = mustHex("#000000")
	}

	accent := strongAccent(text, bgs, secondaryHex)

	hex := func(c colorful.Color) lipgloss.Color { return lipgloss.Color(c.Hex()) }
	p := Palette{
		Accent:    hex(accent),
		OnAccent:  readableTextOn(accent),
		Text:      hex(text),
		Muted:     hex(muted(text, bgs)),
		Border:    hex(accent),
		Positive:  hex(solveOn(mustHex("#2ec27e"), bgs, minContrast)),
		Negative:  hex(solveOn(mustHex("#e01b24"), bgs, minContrast)),
		Draw:      hex(solveOn(mustHex("#F5C04A"), bgs, minContrast)),
		Info:      hex(solveOn(mustHex("#4CC9F0"), bgs, minContrast)),
		Below:     hex(solveOn(mustHex("#FF9F43"), bgs, minContrast)),
		Faint:     hex(muted(text, bgs)),
		Highlight: hex(text),
	}

	t := New(p)
	t.Background, t.Stripe = hex(a), hex(b)
	t.Title = t.Title.Foreground(hex(primary)).Background(hex(text))
	t.TableSelected = lipgloss.NewStyle().Foreground(hex(primary)).Background(hex(text)).Bold(true)
	// Highlight is the text color here, so New's keycap (text on
	// Highlight) and idle tab (muted on Highlight) would vanish: keys are
	// drawn inverse like the title, idle tabs sit unfilled on the screen.
	t.Keycap = t.Keycap.Foreground(hex(primary)).Background(hex(text))
	t.TabIdle = lipgloss.NewStyle().Foreground(p.Muted)
	if worstContrast(accent, bgs) < minContrast {
		// The fallback accent only reads as bold text.
		t.Accent = t.Accent.Bold(true)
	}
	return t
}

// strongAccent is the accent on stripes bgs: the club's secondary if it
// can be made readable there; otherwise whichever of black and white the
// body text isn't, if it reads as bold text (minLargeContrast); and
// only failing both the body text itself. On a bright red neither text
// colour nor a navy secondary reaches 4.5:1, and an accent the same as
// the text would mark nothing.
func strongAccent(text colorful.Color, bgs []colorful.Color, secondaryHex string) colorful.Color {
	if secondary, ok := parseHex(secondaryHex); ok {
		if c := solveOn(secondary, bgs, minContrast); worstContrast(c, bgs) >= minContrast {
			return c
		}
	}
	other := mustHex("#000000")
	if text == other {
		other = mustHex("#ffffff")
	}
	if worstContrast(other, bgs) >= minLargeContrast {
		return other
	}
	return text
}

// worstContrast is c's contrast against whichever of bgs it reads least
// well on.
func worstContrast(c colorful.Color, bgs []colorful.Color) float64 {
	worst := math.Inf(1)
	for _, bg := range bgs {
		worst = min(worst, contrastRatio(c, bg))
	}
	return worst
}

// solveOn adjusts c's lightness until it reads against every one of bgs,
// trying both directions — on a mid-tone background either may be the one
// that works — and keeping whichever gets closer.
func solveOn(c colorful.Color, bgs []colorful.Color, threshold float64) colorful.Color {
	if worstContrast(c, bgs) >= threshold {
		return c
	}
	h, s, l := c.Hsl()
	best, bestRatio := c, worstContrast(c, bgs)
	for _, step := range []float64{0.02, -0.02} {
		for v := l + step; v >= 0 && v <= 1; v += step {
			candidate := quantize(colorful.Hsl(h, s, v))
			r := worstContrast(candidate, bgs)
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
// readable on every stripe, giving secondary text that recedes without
// disappearing.
func muted(text colorful.Color, bgs []colorful.Color) colorful.Color {
	best := text
	for f := 0.05; f < 1; f += 0.05 {
		c := quantize(text.BlendLab(bgs[0], f))
		if worstContrast(c, bgs) < minContrast {
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

// TextOn is black or white, whichever reads on bg, for text drawn on a
// fill of a theme color. An adaptive bg (any weak-theme color) has no
// single value to solve against, and gets fallback.
func TextOn(bg, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	if c, ok := parseHex(colorHex(bg)); ok {
		return readableTextOn(c)
	}
	return fallback
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
