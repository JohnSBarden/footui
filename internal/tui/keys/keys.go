// Package keys defines footui's key bindings, each paired with its help
// text so the footer can't drift from what's handled.
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
	// QuitFromTextEntry is advertised while a text box has focus, where
	// "q" is a literal.
	QuitFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	)
	Help = key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "keys"),
	)
	// Refresh: ctrl+r works while a text box has focus.
	Refresh = key.NewBinding(
		key.WithKeys("r", "ctrl+r"),
		key.WithHelp("r", "refresh"),
	)
	// RefreshFromTextEntry is advertised while a text box has focus.
	RefreshFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+r"),
		key.WithHelp("ctrl+r", "refresh"),
	)
	// Catalog opens the league catalog; ctrl+g works in a text box.
	Catalog = key.NewBinding(
		key.WithKeys("d", "ctrl+g"),
		key.WithHelp("d", "download"),
	)
	// Kickabout is deliberately only in the full help, never the footer.
	Kickabout = key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("ctrl+k", "kickabout"),
	)
	// Theme switches club screens between weak and strong themes.
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
	// Favorite stars or un-stars what's on screen or highlighted.
	Favorite = key.NewBinding(
		key.WithKeys("f", "ctrl+f"),
		key.WithHelp("f", "★ favourite"),
	)
	FavoriteFromTextEntry = key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "★ favourite"),
	)
)

// Log opens and closes the log panel, from any screen.
var Log = key.NewBinding(
	key.WithKeys("L"),
	key.WithHelp("L", "log"),
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
	// LastGame opens a club's last game in full.
	LastGame = key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "last game"),
	)
	// MatchTabs steps through a match's timeline, stats, lineups and
	// commentary, or jumps to one by number.
	MatchTabs = key.NewBinding(
		key.WithKeys("tab", "shift+tab", "[", "]", "1", "2", "3", "4"),
		key.WithHelp("[/]/1-4", "section"),
	)
)

// Navigation bindings screens share; each declares those it handles.
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
	// MoveInTextEntry omits j/k, which a text box needs as letters.
	MoveInTextEntry = key.NewBinding(
		key.WithKeys("up", "down", "ctrl+p", "ctrl+n"),
		key.WithHelp("↑/↓", "move"),
	)
	// ToResults moves focus from the search box to its results.
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
