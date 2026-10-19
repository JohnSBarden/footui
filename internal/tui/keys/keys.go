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
		key.WithHelp("d", "download"),
	)
	// Kickabout turns the screen into a pitch. It's deliberately left out
	// of every footer, a surprise listed only in the full help; ctrl+k
	// rather than a letter so it works while a text box has focus.
	Kickabout = key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("ctrl+k", "kickabout"),
	)
	// Theme switches club screens between the weak theme (club colors as
	// highlights) and the strong one (painted in the club's colors).
	Theme = key.NewBinding(
		key.WithKeys("t", "ctrl+t"),
		key.WithHelp("t", "theme"),
	)
	// ThemeFromTextEntry is the theme key while a text box has focus.
	ThemeFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "theme"),
	)
	// Find returns to the search box from any screen.
	Find = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	)
	CatalogFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+g"),
		key.WithHelp("ctrl+g", "download"),
	)
	// Favorite stars or un-stars the league, club or player on screen (or
	// the highlighted search result or dashboard card). ctrl+f works while
	// a text box has focus.
	Favorite = key.NewBinding(
		key.WithKeys("f", "ctrl+f"),
		key.WithHelp("f", "★ favourite"),
	)
	FavoriteFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "★ favourite"),
	)
)

// MoveCard moves between dashboard cards, in two dimensions.
var MoveCard = key.NewBinding(
	key.WithKeys("up", "down", "left", "right", "h", "j", "k", "l"),
	key.WithHelp("←↑↓→", "move"),
)

// Bindings used by the league catalog.
var (
	Toggle = key.NewBinding(
		key.WithKeys(" ", "x"),
		key.WithHelp("space", "select"),
	)
	SelectMissing = key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "select all missing"),
	)
	SelectCurrent = key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "select current"),
	)
	SelectCurrentEverywhere = key.NewBinding(
		key.WithKeys("C"),
		key.WithHelp("C", "select current, starter leagues"),
	)
	ClearSelection = key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "clear selection"),
	)
	// StopDownloads cancels the running download and empties the queue.
	StopDownloads = key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "stop downloads"),
	)
	Download = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "download"),
	)
	CatalogPane = key.NewBinding(
		key.WithKeys("tab", "left", "right", "h", "l"),
		key.WithHelp("tab/←→", "switch list"),
	)
)

// Bindings used by the nation screen and tournaments.
var (
	Year = key.NewBinding(
		key.WithKeys("left", "right", "h", "l"),
		key.WithHelp("←/→", "year"),
	)
	// FilterBar moves into, and back out of, a nation's competition
	// filter.
	FilterBar = key.NewBinding(
		key.WithKeys("tab", "shift+tab"),
		key.WithHelp("tab", "filter"),
	)
	FilterDone = key.NewBinding(
		key.WithKeys("tab", "shift+tab", "esc"),
		key.WithHelp("tab", "back to matches"),
	)
	FilterChoose = key.NewBinding(
		key.WithKeys("left", "right", "h", "l"),
		key.WithHelp("←/→", "choose"),
	)
	FilterToggle = key.NewBinding(
		key.WithKeys(" ", "x", "enter"),
		key.WithHelp("space", "toggle"),
	)
	// BoxScore expands the highlighted match's team stats in place.
	BoxScore = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "box score"),
	)
	// Group steps through a tournament's group tables.
	Group = key.NewBinding(
		key.WithKeys("[", "]"),
		key.WithHelp("[/]", "group"),
	)
	// Scores scrolls the strip of scores along the top of the screen.
	Scores = key.NewBinding(
		key.WithKeys("<", ">", ",", "."),
		key.WithHelp("</>", "scores"),
	)
	// Matches switches a tournament between its tables and every fixture
	// by round.
	Matches = key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "matches"),
	)
	// Opponent opens the other side of the highlighted match.
	Opponent = key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "opponent"),
	)
	Tables = key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "tables"),
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
	// Move is Up and Down as one footer item; the full help lists them
	// separately with their vim keys.
	Move = key.NewBinding(
		key.WithKeys("up", "down", "k", "j"),
		key.WithHelp("↑/↓", "move"),
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
	// MoveInTextEntry is cursor movement while a text box has focus, where
	// j and k are letters rather than movement.
	MoveInTextEntry = key.NewBinding(
		key.WithKeys("up", "down", "ctrl+p", "ctrl+n"),
		key.WithHelp("↑/↓", "move"),
	)
	// ToResults moves focus from the search box to its results, where
	// bare-letter shortcuts work again.
	ToResults = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "results"),
	)
	// ClearSearch empties a search box that has text in it.
	ClearSearch = key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear"),
	)
)
