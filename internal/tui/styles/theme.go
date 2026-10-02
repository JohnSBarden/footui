// Package styles owns every Lip Gloss style footui renders with. Screens
// never construct styles inline: they read them off a Theme, so that a
// club's theme can be swapped in wholesale and a no-color mode can
// flatten everything in one place.
package styles

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Palette is the raw color set a Theme is derived from. Keeping colors
// separate from styles is what lets TeamTheme rebuild a full Theme from
// just a club's brand colors.
type Palette struct {
	Accent    lipgloss.TerminalColor // primary brand/highlight color
	OnAccent  lipgloss.TerminalColor // text drawn on top of Accent
	Text      lipgloss.TerminalColor
	Muted     lipgloss.TerminalColor
	Border    lipgloss.TerminalColor
	Positive  lipgloss.TerminalColor // over-performance, wins, positive GD
	Negative  lipgloss.TerminalColor // under-performance, losses, negative GD
	Highlight lipgloss.TerminalColor // selected-row background
}

// DefaultPalette is the un-branded palette used before a team's colors are
// known (Browse, League) and whenever a team has no seeded colors.
func DefaultPalette() Palette {
	return Palette{
		Accent:    lipgloss.AdaptiveColor{Light: "#1a5fb4", Dark: "#78aeed"},
		OnAccent:  lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#0b0b0b"},
		Text:      lipgloss.AdaptiveColor{Light: "#1c1c1c", Dark: "#e6e6e6"},
		Muted:     lipgloss.AdaptiveColor{Light: "#6b6b6b", Dark: "#8f8f8f"},
		Border:    lipgloss.AdaptiveColor{Light: "#c4c4c4", Dark: "#4a4a4a"},
		Positive:  lipgloss.AdaptiveColor{Light: "#1c7c3c", Dark: "#5fd77f"},
		Negative:  lipgloss.AdaptiveColor{Light: "#a51d2d", Dark: "#f07178"},
		Highlight: lipgloss.AdaptiveColor{Light: "#dae6f5", Dark: "#2a3a52"},
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
	Positive lipgloss.Style
	Negative lipgloss.Style

	// Panels
	Panel      lipgloss.Style
	PanelTitle lipgloss.Style

	// Background is the color the whole screen is painted in, or nil to
	// leave the terminal's own background showing — every theme except a
	// strong club theme.
	Background lipgloss.TerminalColor

	// NoColor reports whether color was disabled; screens that would
	// otherwise convey meaning through color alone add a text marker.
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
		Positive:      lipgloss.NewStyle().Foreground(p.Positive),
		Negative:      lipgloss.NewStyle().Foreground(p.Negative),
		Panel:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.Border).Padding(0, 1),
		PanelTitle:    lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
	}
}

// Default returns the un-branded theme.
func Default() Theme { return New(DefaultPalette()) }

// Plain returns a theme with every color stripped, for NO_COLOR and
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
	// Without color, the selected row is marked by reverse video — the one
	// highlight every terminal renders regardless of color support.
	t.TableSelected = reverse
	t.Accent = bold
	t.Muted = plain
	t.Positive = plain
	t.Negative = plain
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

// footyBlue and footyGold are footy's own colors, used to paint the
// screens that belong to no club (Browse, a league table, the catalog)
// in strong mode, so switching modes visibly changes every screen.
const (
	footyBlue = "#1A5FB4"
	footyGold = "#F8E45C"
)

// BaseFor is the theme for screens with no club colors of their own, in
// the given mode: the terminal's background in weak mode, footy's blue
// in strong mode. No color still means no color.
func BaseFor(mode Mode, noColorFlag bool) Theme {
	t := Resolve(noColorFlag)
	if t.NoColor || mode != Strong {
		return t
	}
	strong, _ := StrongTeamTheme(footyBlue, footyGold)
	return strong
}
