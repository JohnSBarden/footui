package core

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// RefreshRequest is one unit of refresh work: a competition and the
// seasons of it to fetch.
type RefreshRequest struct {
	CompetitionID string
	// Seasons are bare start years ("2025" for 2025-26), most recent
	// first. Empty means the current season — what the refresh key
	// fetches.
	Seasons []string
	// Why, when set, is shown alongside the progress line so a refresh
	// the user didn't ask for (the stale check on open) explains itself.
	Why string
}

// Refresher fetches one RefreshRequest into the cache. It is how the TUI
// runs the same work as `footy refresh` without importing the provider or
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
type RefreshedMsg struct{ CompetitionID string }

// RefreshRequestMsg asks the root app to queue refresh work. Screens that
// let the user pick what to fetch (the league catalog) send it rather than
// calling a Refresher themselves, so there is only ever one refresh
// running and one place that reports on it.
type RefreshRequestMsg struct{ Requests []RefreshRequest }

// RequestRefresh returns a command that queues reqs with the root app.
func RequestRefresh(reqs ...RefreshRequest) tea.Cmd {
	return func() tea.Msg { return RefreshRequestMsg{Requests: reqs} }
}

// RefreshQueueMsg is broadcast to every screen whenever the refresh queue
// changes, so a screen can mark what is waiting or in flight. Active is
// nil when nothing is running.
type RefreshQueueMsg struct {
	Active  *RefreshRequest
	Pending []RefreshRequest
}

// CatalogEntry is one competition footy knows how to fetch, with every
// season the provider offers for it. It is handed to the TUI in Deps so
// the catalog screen can list what's available without importing the
// sync package that defines coverage.
type CatalogEntry struct {
	Competition models.Competition
	// Seasons are bare start years, most recent first.
	Seasons []string
}
