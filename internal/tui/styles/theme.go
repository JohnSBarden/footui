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
	Faint     lipgloss.TerminalColor // dividers and decoration, never essential text
	Border    lipgloss.TerminalColor
	Positive  lipgloss.TerminalColor // over-performance, wins, positive GD
	Negative  lipgloss.TerminalColor // under-performance, losses, negative GD
	Draw      lipgloss.TerminalColor // draws, honours
	Info      lipgloss.TerminalColor // xG, Champions League places
	Below     lipgloss.TerminalColor // fell short of expectation, Europa places
	Highlight lipgloss.TerminalColor // selected-row background
}

// DefaultPalette is the un-branded palette used before a team's colors are
// known (Browse, League) and whenever a team has no seeded colors. Its
// accent is footui's own purple; every text role clears WCAG AA on both a
// light and a dark terminal.
func DefaultPalette() Palette {
	return Palette{
		Accent:    lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#A66BFF"},
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
		Highlight: lipgloss.AdaptiveColor{Light: "#EEE6FB", Dark: "#221A30"},
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

// baseViolet and baseLavender are footui's own strong-mode colors, used to
// paint the screens that belong to no club (Browse, a league table, the
// catalog), so switching modes visibly changes every screen.
const (
	baseViolet   = "#2E1065"
	baseLavender = "#C9A7FF"
)

// BaseFor is the theme for screens with no club colors of their own, in
// the given mode: the terminal's background in weak mode, footui's violet
// in strong mode. No color still means no color.
func BaseFor(mode Mode, noColorFlag bool) Theme {
	t := Resolve(noColorFlag)
	if t.NoColor || mode != Strong {
		return t
	}
	strong, _ := StrongTeamTheme(baseViolet, baseLavender)
	return strong
}
