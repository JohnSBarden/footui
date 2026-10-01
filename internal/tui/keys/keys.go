// Package keys defines footui's key bindings in one place.
//
// Bindings pair the keys themselves with their help text, so a binding and
// the footer that documents it cannot drift apart — the recurring failure
// mode of hand-written help strings. The root app renders the footer from
// whatever bindings the current screen declares, so no screen writes its
// own.
package keys

import "github.com/charmbracelet/bubbles/key"

// Bindings shared by every screen. The root app handles these itself.
var (
	Back = key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	)
	Quit = key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	)
	// QuitFromTextEntry is what the footer advertises while a screen has a
	// focused text box. There, "q" is a literal character the box needs,
	// so offering it as the quit key would be a lie the user discovers by
	// typing their search and finding nothing happens.
	QuitFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	)
	Help = key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "keys"),
	)
	// Refresh fetches the current screen's competition. ctrl+r is bound
	// too so the key works while a text box has focus.
	Refresh = key.NewBinding(
		key.WithKeys("r", "ctrl+r"),
		key.WithHelp("r", "refresh"),
	)
	// RefreshFromTextEntry is what the footer advertises while a screen
	// has a focused text box, where "r" is a letter the box needs.
	RefreshFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "refresh"),
	)
	// Catalog opens the league catalog, where competitions and seasons
	// are chosen for download. ctrl+g works while a text box has focus.
	Catalog = key.NewBinding(
		key.WithKeys("d", "ctrl+g"),
		key.WithHelp("d", "download leagues"),
	)
	// Theme switches club screens between the weak theme (club colors as
	// highlights) and the strong one (painted in the club's colors).
	Theme = key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "strong/weak theme"),
	)
	CatalogFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "download leagues"),
	)
)

// Bindings used by the league catalog.
var (
	Toggle = key.NewBinding(
		key.WithKeys(" ", "x"),
		key.WithHelp("space", "select season"),
	)
	SelectMissing = key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "select all missing"),
	)
	SelectCurrent = key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "select current"),
	)
	Download = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "download selected"),
	)
	CatalogPane = key.NewBinding(
		key.WithKeys("tab", "left", "right", "h", "l"),
		key.WithHelp("tab/←→", "leagues/seasons"),
	)
)

// Navigation bindings screens share. Each screen picks the ones it
// actually responds to; a binding a screen doesn't declare is not shown.
var (
	Up = key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	)
	Down = key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	)
	PageUp = key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "page up"),
	)
	PageDown = key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "page down"),
	)
	Home = key.NewBinding(
		key.WithKeys("home", "g"),
		key.WithHelp("g", "first"),
	)
	End = key.NewBinding(
		key.WithKeys("end", "G"),
		key.WithHelp("G", "last"),
	)
	Open = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open"),
	)
	Season = key.NewBinding(
		key.WithKeys("left", "right", "h", "l"),
		key.WithHelp("←/→", "season"),
	)
	Competition = key.NewBinding(
		key.WithKeys("left", "right", "h", "l"),
		key.WithHelp("←/→", "competition"),
	)
	Pane = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "seasons/squad"),
	)
	Search = key.NewBinding(
		key.WithKeys("ctrl+n", "ctrl+p"),
		key.WithHelp("type", "search"),
	)
)
