package styles

import (
	"math"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Brand colours are never used raw: the terminal background is unknown, so
// each is adjusted for light and dark and handed over as an AdaptiveColor.
const (
	// minContrast is the WCAG AA ratio for normal-size text.
	minContrast = 4.5
	// minLargeContrast is WCAG AA for large text; holding marks and bold
	// text to 4.5 would wash out most club primaries.
	minLargeContrast = 3.0

	// Realistic terminal extremes; few are pure white or black.
	lightBackground = "#ffffff"
	darkBackground  = "#0b0b0b"
)

// TeamPalette derives a readable palette from a club's brand colours.
// secondary and tertiary may be empty; the primary alone is enough.
func TeamPalette(primaryHex, secondaryHex string) Palette {
	base := applyAccent(DefaultPalette())

	primary, ok := parseHex(primaryHex)
	if !ok {
		return base
	}

	base.Accent = adaptiveForeground(primary, minContrast)
	base.OnAccent = readableTextOn(primary)
	base.Highlight = selectionBackground(primary)

	if secondary, ok := parseHex(secondaryHex); ok {
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

// HeroStyles are a club screen's header band and the rule beneath it. In
// strong mode the screen already is the primary, so the band inverts.
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

// TeamTheme builds a full Theme from a club's brand colours.
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

// StrongTeamTheme builds a theme whose background is the club's primary,
// cut into mowing stripes. The background is known exactly, so colours are
// solved against both stripes rather than adapted per terminal.
func StrongTeamTheme(primaryHex, secondaryHex string) (Theme, bool) {
	primary, ok := parseHex(primaryHex)
	if !ok {
		return Theme{}, false
	}
	return strongTheme(primary, primary, mowingShade(primary), secondaryHex), true
}

// stripeSpread is the HSL lightness gap between the two mowing shades,
// about the gap between the kickabout's two greens.
const stripeSpread = 0.044

// mowingShade moves the primary away from its text colour (darker under
// white, lighter under black) so the cut never costs contrast.
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

// strongTheme builds a strong theme on stripes a and b. primary is drawn
// on inverse elements (title bar, keys, selected row).
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
	t.Shade = hex(panelShade(a, text))
	t.Title = t.Title.Foreground(hex(primary)).Background(hex(text))
	t.TableSelected = lipgloss.NewStyle().Foreground(hex(primary)).Background(hex(text)).Bold(true)
	// Highlight is the text colour here, so New's keycap (text on
	// Highlight) and idle tab (muted on Highlight) would vanish.
	t.Keycap = t.Keycap.Foreground(hex(primary)).Background(hex(text))
	t.TabIdle = lipgloss.NewStyle().Foreground(p.Muted)
	if worstContrast(accent, bgs) < minContrast {
		// The fallback accent only reads as bold text.
		t.Accent = t.Accent.Bold(true)
	}
	return t
}

// panelShade moves away from the text so the fill only adds contrast.
func panelShade(a, text colorful.Color) colorful.Color {
	h, s, l := a.Hsl()
	if text.Hex() == "#ffffff" {
		return quantize(colorful.Hsl(h, s, max(l-2*stripeSpread, 0)))
	}
	return quantize(colorful.Hsl(h, s, min(l+2*stripeSpread, 1)))
}

// strongAccent is the secondary if readable on bgs, else the opposite of
// the body text if it reads as bold, else the body text itself.
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

func worstContrast(c colorful.Color, bgs []colorful.Color) float64 {
	worst := math.Inf(1)
	for _, bg := range bgs {
		worst = min(worst, contrastRatio(c, bg))
	}
	return worst
}

// solveOn adjusts c's lightness until it reads on every bg, trying both
// directions since either may work on a mid-tone.
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

// quantize rounds to the rendered 8-bit hex; contrast must be checked on
// this, or a float blend that passes can fail once written out.
func quantize(c colorful.Color) colorful.Color {
	return mustHex(c.Clamped().Hex())
}

func colorDistance(a, b colorful.Color) float64 { return a.DistanceLab(b) }

// muted blends text toward the background while staying readable.
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

// parseHex accepts "#rrggbb" or "rrggbb".
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

// adaptiveForeground moves only HSL lightness, keeping the club's hue.
func adaptiveForeground(c colorful.Color, threshold float64) lipgloss.TerminalColor {
	return lipgloss.AdaptiveColor{
		Light: adjustForBackground(c, mustHex(lightBackground), threshold).Hex(),
		Dark:  adjustForBackground(c, mustHex(darkBackground), threshold).Hex(),
	}
}

func adjustForBackground(c, bg colorful.Color, threshold float64) colorful.Color {
	if contrastRatio(c, bg) >= threshold {
		return c
	}

	h, s, l := c.Hsl()
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
	// Nothing cleared the threshold; the most extreme is closest.
	return best
}

// readableTextOn picks black or white, whichever contrasts more.
func readableTextOn(bg colorful.Color) lipgloss.TerminalColor {
	black, white := mustHex("#000000"), mustHex("#ffffff")
	if contrastRatio(white, bg) >= contrastRatio(black, bg) {
		return lipgloss.Color("#ffffff")
	}
	return lipgloss.Color("#000000")
}

// TextOn is black or white, whichever reads on bg. An adaptive bg has no
// single value to solve against, and gets fallback.
func TextOn(bg, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	if c, ok := parseHex(colorHex(bg)); ok {
		return readableTextOn(c)
	}
	return fallback
}

// selectionBackground tints the row without competing with its text.
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

// relativeLuminance is the WCAG 2.x relative luminance of a colour.
func relativeLuminance(c colorful.Color) float64 {
	channel := func(v float64) float64 {
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

// contrastRatio is the WCAG contrast ratio between two colours, from 1
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
		// Only called with literals; panicking beats silently returning black.
		panic("styles: bad built-in color " + hex)
	}
	return c
}
