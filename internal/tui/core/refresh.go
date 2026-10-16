package core

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// RefreshRequest is one unit of refresh work. Usually that is a
// competition and the seasons of it to fetch; it can instead be a
// national team's matches (TeamID), every nation's matches of one gender
// (Nations), or the directory of national teams (Directory).
type RefreshRequest struct {
	CompetitionID string
	// TeamID asks for a national team's matches: the shared yearly match
	// lists of every international competition of its gender, which
	// cover every other nation of that gender too.
	TeamID string
	// Nations ("men" or "women") asks for those yearly match lists
	// without naming a team — the catalog's national teams line.
	Nations string
	// Directory asks for the list of national teams, so search can find
	// every nation before any of its matches are fetched.
	Directory bool
	// Seasons are bare start years ("2025" for 2025-26), most recent
	// first. Empty means the current season — what the refresh key
	// fetches. For TeamID and Nations they are calendar years, and empty
	// means this year and last.
	Seasons []string
	// Name is how the status line refers to the request, when the ID
	// alone would read badly ("England" rather than "nat-england").
	Name string
	// Why, when set, is shown alongside the progress line so a refresh
	// the user didn't ask for (the stale check on open) explains itself.
	Why string
	// Automatic marks a refresh nobody asked for, which is held to the
	// stale threshold; one the user asked for fetches whatever has
	// changed since.
	Automatic bool
}

// Key identifies what a request fetches, so two requests for the same
// thing can be merged.
func (r RefreshRequest) Key() string {
	switch {
	case r.Directory:
		return "directory"
	case r.TeamID != "":
		return "team:" + r.TeamID
	case r.Nations != "":
		return "nations:" + r.Nations
	}
	return "competition:" + r.CompetitionID
}

// Label is how the status line names the request.
func (r RefreshRequest) Label() string {
	switch {
	case r.Name != "":
		return r.Name
	case r.Directory:
		return "national teams"
	case r.TeamID != "":
		return r.TeamID
	case r.Nations != "":
		return "national teams (" + r.Nations + ")"
	}
	return r.CompetitionID
}

// ErrRateLimited is what a Refresher returns when the source has asked
// footui to stop for a while. The root app then drops everything queued
// rather than carrying on asking.
var ErrRateLimited = errors.New("ESPN is rate-limiting footui; try again later")

// RefreshRequester is implemented by screens whose refresh isn't a single
// competition — a nation's — and so hand the root app a whole request. It
// is asked before Refreshable.
type RefreshRequester interface {
	RefreshRequest() (RefreshRequest, bool)
}

// Refresher fetches one RefreshRequest into the cache. It is how the TUI
// runs the same work as `footui refresh` without importing the provider or
// sync packages: the root app is handed one in Deps and screens never see
// it.
//
// progress is called with a human-readable line each time something worth
// telling the user happens mid-fetch (a season finishing, a retry), since
// a refresh against a slow host can take most of a minute and silence for
// that long looks like a hang. The returned summary is one line describing
// what was cached, shown once the refresh succeeds.
type Refresher func(ctx context.Context, req RefreshRequest, progress func(text string)) (summary string, err error)

// Refreshable is implemented by screens that know which competition a
// refresh triggered from them should fetch. A screen with nothing sensible
// to refresh (a player, say) simply doesn't implement it, and the root app
// explains that instead of guessing.
type Refreshable interface {
	RefreshTarget() (competitionID string, ok bool)
}

// RefreshedMsg is broadcast by the root app to every screen on the stack
// after a refresh succeeds, so each can re-query and replace its empty
// state with the data that just arrived — including screens beneath the
// visible one, which the user may pop back to.
type RefreshedMsg struct {
	CompetitionID string
	// TeamID is the team whose matches were fetched: a nation, or a club's
	// recent games.
	TeamID string
	// International is set when what landed was international matches
	// (a nation's, or every nation's of a gender), which any screen
	// showing a nation or a tournament may want to re-read.
	International bool
}

// RefreshRequestMsg asks the root app to queue refresh work. Screens that
// let the user pick what to fetch (the league catalog) send it rather than
// calling a Refresher themselves, so there is only ever one refresh
// running and one place that reports on it.
type RefreshRequestMsg struct{ Requests []RefreshRequest }

// RequestRefresh returns a command that queues reqs with the root app.
func RequestRefresh(reqs ...RefreshRequest) tea.Cmd {
	return func() tea.Msg { return RefreshRequestMsg{Requests: reqs} }
}

// CancelRefreshMsg asks the root app to stop the running refresh and drop
// everything queued behind it.
type CancelRefreshMsg struct{}

// CancelRefresh returns a command that cancels all refresh work.
func CancelRefresh() tea.Cmd {
	return func() tea.Msg { return CancelRefreshMsg{} }
}

// RefreshQueueMsg is broadcast to every screen whenever the refresh queue
// changes, so a screen can mark what is waiting or in flight. Active is
// nil when nothing is running.
type RefreshQueueMsg struct {
	Active  *RefreshRequest
	Pending []RefreshRequest
}

// CatalogEntry is one competition footui knows how to fetch, with every
// season the provider offers for it. It is handed to the TUI in Deps so
// the catalog screen can list what's available without importing the
// sync package that defines coverage.
type CatalogEntry struct {
	Competition models.Competition
	// Seasons are bare start years, most recent first.
	Seasons []string
	// ByYear marks an international competition offered by calendar
	// year (a qualifier, a nations league): each season is one of its
	// shared yearly match lists, rather than an edition.
	ByYear bool
	// Nations, "men" or "women", marks the national teams line: each
	// season is a calendar year of every international competition of
	// that gender — what every nation's matches that year come from.
	// Competition is then only a label.
	Nations string
	// Units lists, for the national teams line, the competitions whose
	// match lists make up each year, so the catalog can tell which years
	// are already fetched.
	Units map[string][]string
}
