// Package styles owns every Lip Gloss style footui renders with. Screens
// never construct styles inline: they read them off a Theme, so that a
// club's theme can be swapped in wholesale and a no-color mode can
// flatten everything in one place.
package styles

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Palette is the raw colour set a Theme is derived from. Keeping colours
// separate from styles is what lets TeamTheme rebuild a full Theme from
// just a club's brand colours.
type Palette struct {
	Accent    lipgloss.TerminalColor // primary brand/highlight colour
	OnAccent  lipgloss.TerminalColor // text drawn on top of Accent
	Text      lipgloss.TerminalColor
	Muted     lipgloss.TerminalColor
	Faint     lipgloss.TerminalColor // dividers and decoration, never essential text
	Border    lipgloss.TerminalColor
	Positive  lipgloss.TerminalColor // over-performance, wins, positive GD
	Negative  lipgloss.TerminalColor // under-performance, losses, negative GD
	Draw      lipgloss.TerminalColor // draws, honours
	Info      lipgloss.TerminalColor // xG, Champions League places
	Below     lipgloss.TerminalColor // fell short of expectation, Europa places
	Highlight lipgloss.TerminalColor // selected-row background
}

// DefaultPalette is the un-branded palette used before a team's colours are
// known (Browse, League) and whenever a team has no seeded colours. Its
// accent is footui's own lime; every text role clears WCAG AA on both a
// light and a dark terminal.
func DefaultPalette() Palette {
	return Palette{
		Accent:    lipgloss.AdaptiveColor{Light: "#4D7C0F", Dark: "#C8F25A"},
		OnAccent:  lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#0b0b0b"},
		Text:      lipgloss.AdaptiveColor{Light: "#1C1F1D", Dark: "#E8EDE6"},
		Muted:     lipgloss.AdaptiveColor{Light: "#5F6B63", Dark: "#8A968E"},
		Faint:     lipgloss.AdaptiveColor{Light: "#9AA59D", Dark: "#5E6A62"},
		Border:    lipgloss.AdaptiveColor{Light: "#c4c4c4", Dark: "#3A463F"},
		Positive:  lipgloss.AdaptiveColor{Light: "#15803D", Dark: "#5BE49B"},
		Negative:  lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#FF6F61"},
		Draw:      lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#F5C04A"},
		Info:      lipgloss.AdaptiveColor{Light: "#0369A1", Dark: "#4CC9F0"},
		Below:     lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FF9F43"},
		Highlight: lipgloss.AdaptiveColor{Light: "#E6EEE3", Dark: "#1C241F"},
	}
}

// Theme is the set of styles screens render with.
type Theme struct {
	Palette Palette

	// Chrome
	Title      lipgloss.Style // app/screen title in the header bar
	Subtitle   lipgloss.Style // secondary header text (country, tier, season)
	Breadcrumb lipgloss.Style
	Help       lipgloss.Style // key hints in the footer
	Status     lipgloss.Style // transient footer message
	Error      lipgloss.Style
	Empty      lipgloss.Style // "no data cached" states

	// Tables
	TableHeader   lipgloss.Style
	TableCell     lipgloss.Style
	TableSelected lipgloss.Style

	// Emphasis
	Accent   lipgloss.Style
	Muted    lipgloss.Style
	Faint    lipgloss.Style
	Positive lipgloss.Style
	Negative lipgloss.Style
	Draw     lipgloss.Style
	Info     lipgloss.Style
	Below    lipgloss.Style

	// Label is a section heading (callers upper-case the text); Keycap a
	// key in the footer; TabActive and TabIdle the two kinds of tab pill.
	Label     lipgloss.Style
	Keycap    lipgloss.Style
	TabActive lipgloss.Style
	TabIdle   lipgloss.Style

	// Panels
	Panel      lipgloss.Style
	PanelTitle lipgloss.Style

	// Background is the colour the whole screen is painted in, or nil to
	// leave the terminal's own background showing — every theme except a
	// strong club theme.
	Background lipgloss.TerminalColor
	// Stripe is the second mowing shade, alternating with Background every
	// few columns like a cut pitch; nil paints Background flat.
	Stripe lipgloss.TerminalColor
	// Shade is the background a selected panel is filled with: the
	// highlight, except on a strong theme, whose highlight is its text
	// colour (selection there is drawn inverse).
	Shade lipgloss.TerminalColor

	// NoColor reports whether colour was disabled; screens that would
	// otherwise convey meaning through colour alone add a text marker.
	NoColor bool
}

// New builds a Theme from a Palette.
func New(p Palette) Theme {
	return Theme{
		Palette:       p,
		Title:         lipgloss.NewStyle().Bold(true).Foreground(p.OnAccent).Background(p.Accent).Padding(0, 1),
		Subtitle:      lipgloss.NewStyle().Foreground(p.Muted),
		Breadcrumb:    lipgloss.NewStyle().Foreground(p.Muted),
		Help:          lipgloss.NewStyle().Foreground(p.Muted),
		Status:        lipgloss.NewStyle().Foreground(p.Accent),
		Error:         lipgloss.NewStyle().Bold(true).Foreground(p.Negative),
		Empty:         lipgloss.NewStyle().Foreground(p.Muted).Italic(true),
		TableHeader:   lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		TableCell:     lipgloss.NewStyle().Foreground(p.Text),
		TableSelected: lipgloss.NewStyle().Foreground(p.Text).Background(p.Highlight).Bold(true),
		Accent:        lipgloss.NewStyle().Foreground(p.Accent),
		Muted:         lipgloss.NewStyle().Foreground(p.Muted),
		Faint:         lipgloss.NewStyle().Foreground(p.Faint),
		Positive:      lipgloss.NewStyle().Foreground(p.Positive),
		Negative:      lipgloss.NewStyle().Foreground(p.Negative),
		Draw:          lipgloss.NewStyle().Foreground(p.Draw),
		Info:          lipgloss.NewStyle().Foreground(p.Info),
		Below:         lipgloss.NewStyle().Foreground(p.Below),
		Label:         lipgloss.NewStyle().Bold(true).Foreground(p.Muted),
		Keycap:        lipgloss.NewStyle().Foreground(p.Text).Background(p.Highlight).Padding(0, 1),
		TabActive:     lipgloss.NewStyle().Bold(true).Foreground(p.OnAccent).Background(p.Accent),
		TabIdle:       lipgloss.NewStyle().Foreground(p.Muted).Background(p.Highlight),
		Panel:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Border).Padding(0, 1),
		PanelTitle:    lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		Shade:         p.Highlight,
	}
}

// Default returns the un-branded theme.
func Default() Theme { return New(applyAccent(DefaultPalette())) }

// Plain returns a theme with every colour stripped, for NO_COLOR and
// --no-color. Layout, padding, and bold/italic are kept: they still convey
// structure on a monochrome terminal, and Lip Gloss drops the attributes
// the terminal can't render anyway.
func Plain() Theme {
	t := Theme{NoColor: true}
	bold := lipgloss.NewStyle().Bold(true)
	plain := lipgloss.NewStyle()
	reverse := lipgloss.NewStyle().Reverse(true)

	t.Palette = DefaultPalette()
	t.Title = bold.Padding(0, 1)
	t.Subtitle = plain
	t.Breadcrumb = plain
	t.Help = plain
	t.Status = plain
	t.Error = bold
	t.Empty = lipgloss.NewStyle().Italic(true)
	t.TableHeader = bold
	t.TableCell = plain
	// Without colour, the selected row is marked by reverse video — the one
	// highlight every terminal renders regardless of colour support.
	t.TableSelected = reverse
	t.Accent = bold
	t.Muted = plain
	t.Faint = plain
	t.Positive = plain
	t.Negative = plain
	t.Draw = plain
	t.Info = plain
	t.Below = plain
	t.Label = bold
	t.Keycap = reverse
	t.TabActive = bold
	t.TabIdle = plain
	t.Panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	t.PanelTitle = bold
	return t
}

// Resolve picks the theme to start the app with. An explicit --no-color
// flag wins; otherwise the NO_COLOR convention (any non-empty value) is
// honoured. See https://no-color.org.
func Resolve(noColorFlag bool) Theme {
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		return Plain()
	}
	return Default()
}

// The pitch is footui's own strong-mode look, used to paint the screens
// that belong to no club (Browse, a league table, the catalog), so
// switching modes visibly changes every screen: the kickabout's two
// mowing greens, with the lime accent on top.
const (
	grassDark  = "#2F6B3A"
	grassLight = "#367A41"
	baseLime   = "#C8F25A"
)

// BaseFor is the theme for screens with no club colours of their own, in
// the given mode: the terminal's background in weak mode, the pitch in
// strong mode. No colour still means no colour.
func BaseFor(mode Mode, noColorFlag bool) Theme {
	t := Resolve(noColorFlag)
	if t.NoColor || mode != Strong {
		return t
	}
	return strongTheme(mustHex(grassDark), mustHex(grassDark), mustHex(grassLight), strongAccentHex())
}
