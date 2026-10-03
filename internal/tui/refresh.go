package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// refreshEvent is one thing a running refresh has to say: a progress line
// while it runs, or (done) its outcome.
type refreshEvent struct {
	progress string
	done     bool
	summary  string
	err      error
}

// refreshMsg delivers one refreshEvent to the app together with the
// channel it came from, so the app can wait for the next one. This is the
// standard Bubble Tea shape for a long-running job that reports as it
// goes: each message re-arms a command that blocks on the channel.
type refreshMsg struct {
	event  refreshEvent
	events <-chan refreshEvent
}

// runRefresh starts req and returns its first event. The goroutine is
// started inside the command rather than in Update so a test can call the
// command synchronously and drive the chain by hand.
func runRefresh(ctx context.Context, refresh core.Refresher, req core.RefreshRequest) tea.Cmd {
	return func() tea.Msg {
		events := make(chan refreshEvent, 16)
		go func() {
			defer close(events)
			summary, err := refresh(ctx, req, func(text string) {
				events <- refreshEvent{progress: text}
			})
			events <- refreshEvent{done: true, summary: summary, err: err}
		}()
		return awaitRefresh(events)()
	}
}

func awaitRefresh(events <-chan refreshEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return refreshMsg{event: event, events: events}
	}
}

// refreshTarget reports which competition a refresh from the current
// screen would fetch, if the screen has one.
func (a *App) refreshTarget() (string, bool) {
	if r, ok := a.current().(core.Refreshable); ok {
		return r.RefreshTarget()
	}
	return "", false
}

// refreshBinding is the refresh key the footer should advertise for the
// current screen, or false when a refresh from here would do nothing.
func (a *App) refreshBinding() (key.Binding, bool) {
	if _, ok := a.refreshTarget(); !ok {
		return key.Binding{}, false
	}
	if a.capturingText() {
		return keys.RefreshFromTextEntry, true
	}
	return keys.Refresh, true
}

// startRefresh handles the refresh key. Every way it can decline is
// explained in the status line: the key is advertised in the footer, so
// pressing it and seeing nothing happen would be a bug from the user's
// side of the screen.
func (a *App) startRefresh() tea.Cmd {
	if a.active != nil {
		return nil
	}
	compID, ok := a.refreshTarget()
	if !ok {
		a.status = "nothing to refresh from here — open a league to refresh it"
		return nil
	}
	if a.deps.Refresh == nil {
		a.status = fmt.Sprintf("refresh isn't available in this view — run  footy refresh --competition %s", compID)
		return nil
	}
	return a.enqueue(core.RefreshRequest{CompetitionID: compID})
}

// enqueue adds requests to the refresh queue and starts the first if
// nothing is running. A request for a competition already waiting is
// folded into that entry, so selecting more seasons of a league while it
// is queued doesn't fetch it twice.
func (a *App) enqueue(reqs ...core.RefreshRequest) tea.Cmd {
	if a.deps.Refresh == nil {
		a.status = "downloading isn't available in this view — run  footy refresh"
		return nil
	}
	for _, req := range reqs {
		a.queue = mergeRequest(a.queue, req)
	}
	if a.active != nil {
		return a.broadcastQueue()
	}
	return a.startNext()
}

// mergeRequest appends req to queue, or merges its seasons into a waiting
// request for the same competition.
func mergeRequest(queue []core.RefreshRequest, req core.RefreshRequest) []core.RefreshRequest {
	for i, q := range queue {
		if q.CompetitionID != req.CompetitionID {
			continue
		}
		// An empty season list means "the current season", which isn't
		// a year that can be merged with others; keep such requests
		// separate rather than guess which year that is.
		if len(q.Seasons) == 0 || len(req.Seasons) == 0 {
			continue
		}
		for _, s := range req.Seasons {
			if !slices.Contains(q.Seasons, s) {
				queue[i].Seasons = append(queue[i].Seasons, s)
			}
		}
		slices.Sort(queue[i].Seasons)
		slices.Reverse(queue[i].Seasons)
		return queue
	}
	return append(queue, req)
}

// startNext pops the next request off the queue and runs it.
func (a *App) startNext() tea.Cmd {
	if len(a.queue) == 0 {
		a.active = nil
		return a.broadcastQueue()
	}
	if a.active == nil {
		// Starting from idle: a new batch, so the last one's outcome is
		// no longer news.
		a.err = nil
		a.status = ""
	}
	req := a.queue[0]
	a.queue = a.queue[1:]
	a.active = &req
	a.refreshing = a.describe(req, "")
	parent := a.deps.Ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.stopActive = cancel
	return tea.Batch(a.broadcastQueue(), runRefresh(ctx, a.deps.Refresh, req))
}

// cancelRefresh drops everything queued and stops the running refresh.
// The running one winds down on its own goroutine; its final event is
// what returns the app to idle.
func (a *App) cancelRefresh() tea.Cmd {
	if a.active == nil {
		a.status = "nothing is downloading"
		return nil
	}
	a.queue = nil
	a.stopping = true
	a.stopActive()
	a.refreshing = "Stopping " + a.active.CompetitionID + "…"
	return a.broadcastQueue()
}

// describe is the status line for the active request: what is being
// fetched, why if it wasn't asked for, and how much is still waiting.
func (a *App) describe(req core.RefreshRequest, progress string) string {
	text := progress
	if text == "" {
		text = "Refreshing " + req.CompetitionID
		if n := len(req.Seasons); n > 1 {
			text += fmt.Sprintf(" (%d seasons)", n)
		}
		if req.Why != "" {
			text += " — " + req.Why
		}
		text += "…"
	}
	if n := len(a.queue); n > 0 {
		text += fmt.Sprintf("  [%d more queued]", n)
	}
	return text
}

// broadcastQueue tells every screen what is queued and running.
func (a *App) broadcastQueue() tea.Cmd {
	msg := core.RefreshQueueMsg{Active: a.active, Pending: slices.Clone(a.queue)}
	return a.broadcast(msg)
}

// broadcast hands msg to every screen on the stack, not just the visible
// one, so a screen the user pops back to isn't showing stale state.
func (a *App) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, s := range a.stack {
		updated, cmd := s.Update(msg)
		a.stack[i] = updated
		cmds = append(cmds, a.routeFrom(i, cmd))
	}
	return tea.Batch(cmds...)
}

// handleRefreshEvent applies one event from the running refresh.
func (a *App) handleRefreshEvent(msg refreshMsg) tea.Cmd {
	ev := msg.event
	if a.active == nil {
		return nil
	}
	req := *a.active
	if !ev.done {
		if !a.stopping {
			a.refreshing = a.describe(req, ev.progress)
		}
		return awaitRefresh(msg.events)
	}

	a.refreshing = ""
	a.stopActive()
	if a.stopping {
		// What finished before the stop is cached and worth showing; the
		// error the cancellation caused is not news.
		a.stopping = false
		a.batch = refreshBatch{}
		a.active = nil
		a.status = "Downloads stopped."
		return tea.Batch(a.broadcast(core.RefreshedMsg{CompetitionID: req.CompetitionID}), a.broadcastQueue())
	}
	var reload tea.Cmd
	if ev.err != nil {
		a.batch.failed = append(a.batch.failed, fmt.Sprintf("%s (%v)", req.CompetitionID, ev.err))
		a.batch.lastErr = ev.err
	} else {
		a.batch.done = append(a.batch.done, req.CompetitionID)
		a.batch.lastSummary = "Refreshed " + req.CompetitionID + ": " + ev.summary
		reload = a.broadcast(core.RefreshedMsg{CompetitionID: req.CompetitionID})
	}

	// A failure doesn't abandon the rest of the queue: each request is a
	// different competition, and one league's trouble says little about
	// the next.
	if len(a.queue) == 0 {
		a.reportBatch()
	}
	return tea.Batch(reload, a.startNext())
}

// refreshBatch accumulates outcomes while the queue drains, so the status
// line can sum up the whole run once it's idle again.
type refreshBatch struct {
	done        []string
	failed      []string
	lastSummary string
	lastErr     error
}

// reportBatch puts the outcome of everything run since the queue was last
// idle on the status line. A lone refresh reports exactly as it always
// has; a batch is summed up so an early failure isn't hidden by a later
// success.
func (a *App) reportBatch() {
	b := a.batch
	a.batch = refreshBatch{}
	switch {
	case len(b.failed) == 0 && len(b.done) == 1:
		a.status = b.lastSummary
	case len(b.failed) == 0:
		a.status = fmt.Sprintf("Refreshed %d competitions: %s", len(b.done), strings.Join(b.done, ", "))
	case len(b.done) == 0 && len(b.failed) == 1:
		a.err = b.lastErr
	default:
		a.err = fmt.Errorf("refreshed %d of %d — failed: %s",
			len(b.done), len(b.done)+len(b.failed), strings.Join(b.failed, "; "))
	}
}
