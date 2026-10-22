package core

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// RefreshRequest is one unit of refresh work: a competition's seasons by
// default, or one of TeamID, Nations, Directory, PlayerID.
type RefreshRequest struct {
	CompetitionID string
	// TeamID fetches the shared yearly match lists of its gender, which
	// cover every other nation of that gender too.
	TeamID string
	// Nations ("men" or "women") fetches those lists without a team.
	Nations string
	// Directory fetches the list of national teams, for search.
	Directory bool
	// PlayerID fetches a player's season detail; when Automatic, the
	// current season only once it's past the stale threshold.
	PlayerID string
	// History, with PlayerID or a club's TeamID, fetches every season of
	// every league they appear in; finished seasons already fetched are
	// kept.
	History bool
	// Seasons are bare start years ("2025" for 2025-26), newest first;
	// empty means current. For TeamID and Nations they are calendar
	// years, and empty means this year and last.
	Seasons []string
	// Name overrides the ID in the status line.
	Name string
	// Why is shown with the progress line, for refreshes nobody asked for.
	Why string
	// Automatic holds the refresh to the stale threshold.
	Automatic bool
}

// Key identifies what a request fetches, so duplicates can be merged.
func (r RefreshRequest) Key() string {
	switch {
	case r.Directory:
		return "directory"
	case r.PlayerID != "":
		return "player:" + r.PlayerID
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
	case r.PlayerID != "":
		return r.PlayerID
	case r.TeamID != "":
		return r.TeamID
	case r.Nations != "":
		return "national teams (" + r.Nations + ")"
	}
	return r.CompetitionID
}

// ErrRateLimited makes the root app drop everything queued.
var ErrRateLimited = errors.New("refreshes are paused: too many requests; try again later")

// RefreshRequester is for screens whose refresh isn't one competition. It
// is asked before Refreshable.
type RefreshRequester interface {
	RefreshRequest() (RefreshRequest, bool)
}

// Refresher fetches one RefreshRequest into the cache, so the TUI needn't
// import provider or sync. progress reports mid-fetch lines, since a slow
// refresh can take a minute; summary is one line shown on success.
type Refresher func(ctx context.Context, req RefreshRequest, progress func(text string)) (summary string, err error)

// Refreshable is for screens that know which competition to refresh.
type Refreshable interface {
	RefreshTarget() (competitionID string, ok bool)
}

// RefreshedMsg is broadcast to every screen on the stack after a refresh
// succeeds, so each can re-query.
type RefreshedMsg struct {
	CompetitionID string
	// TeamID is a nation, or a club whose recent games were fetched.
	TeamID string
	// International is set when international matches landed.
	International bool
	PlayerID      string
}

// RefreshRequestMsg asks the root app to queue refresh work, so only one
// refresh ever runs.
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

// RefreshQueueMsg is broadcast whenever the refresh queue changes. Active
// is nil when nothing is running.
type RefreshQueueMsg struct {
	Active  *RefreshRequest
	Pending []RefreshRequest
}

// CatalogEntry is one fetchable competition and its seasons, so the
// catalog screen needn't import sync.
type CatalogEntry struct {
	Competition models.Competition
	// Seasons are bare start years, most recent first.
	Seasons []string
	// ByYear: each season is a calendar year's match list, not an edition.
	ByYear bool
	// Base marks the starter set, which the catalog's C selects.
	Base bool
	// Nations ("men" or "women") marks the national teams line; each
	// season is a calendar year and Competition is only a label.
	Nations string
	// Units lists, per year, the competitions whose match lists make it
	// up, so the catalog can tell which years are fetched.
	Units map[string][]string
}
