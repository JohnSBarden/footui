package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// refreshEvent is a progress line, or (done) the outcome.
type refreshEvent struct {
	progress string
	done     bool
	summary  string
	err      error
}

// refreshMsg carries its channel so the app can re-arm a command that
// blocks on the next event.
type refreshMsg struct {
	event  refreshEvent
	events <-chan refreshEvent
}

// runRefresh starts the goroutine inside the command, not in Update, so a
// test can drive the chain synchronously.
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

// refreshTarget reports what a refresh from the current screen would fetch.
func (a *App) refreshTarget() (core.RefreshRequest, bool) {
	if r, ok := a.current().(core.RefreshRequester); ok {
		if req, ok := r.RefreshRequest(); ok {
			return req, true
		}
	}
	if r, ok := a.current().(core.Refreshable); ok {
		if id, ok := r.RefreshTarget(); ok {
			return core.RefreshRequest{CompetitionID: id}, true
		}
	}
	return core.RefreshRequest{}, false
}

// refreshBinding is false when a refresh from here would do nothing.
func (a *App) refreshBinding() (key.Binding, bool) {
	if _, ok := a.refreshTarget(); !ok {
		return key.Binding{}, false
	}
	if a.capturingText() {
		return keys.RefreshFromTextEntry, true
	}
	return keys.Refresh, true
}

// startRefresh explains every way it declines in the status line, since
// the key is advertised.
func (a *App) startRefresh() tea.Cmd {
	if a.active != nil {
		return nil
	}
	req, ok := a.refreshTarget()
	if !ok {
		a.status = "nothing to refresh from here — open a league to refresh it"
		return nil
	}
	if a.deps.Refresh == nil {
		if req.TeamID != "" {
			a.status = "refresh isn't available in this view — run  footui refresh --team " + req.Label()
		} else {
			a.status = fmt.Sprintf("refresh isn't available in this view — run  footui refresh --competition %s", req.CompetitionID)
		}
		return nil
	}
	return a.enqueue(req)
}

// enqueue queues reqs, folding duplicates of waiting entries, and starts
// the first if idle.
func (a *App) enqueue(reqs ...core.RefreshRequest) tea.Cmd {
	if a.deps.Refresh == nil {
		a.status = "downloading isn't available in this view — run  footui refresh"
		return nil
	}
	for _, req := range reqs {
		if req.PlayerID != "" {
			// Dedupe players. A lone request is a player being opened, so
			// it jumps the queue; a History request isn't answered by a
			// detail-only one.
			if a.active != nil && a.active.Key() == req.Key() && (!req.History || a.active.History) {
				continue
			}
			a.queue = slices.DeleteFunc(a.queue, func(q core.RefreshRequest) bool {
				if q.Key() != req.Key() {
					return false
				}
				req = upgrade(req, q)
				return true
			})
			if len(reqs) == 1 {
				a.queue = append([]core.RefreshRequest{req}, a.queue...)
			} else {
				a.queue = append(a.queue, req)
			}
			continue
		}
		if a.active != nil && len(mergeRequest([]core.RefreshRequest{*a.active}, req)) == 1 && len(req.Seasons) == 0 &&
			(req.TeamID != "" || req.Nations != "" || req.Directory) && (!req.History || a.active.History) {
			continue
		}
		a.queue = mergeRequest(a.queue, req)
	}
	for _, req := range reqs {
		a.deps.Log.Add(core.LogInfo, "queue "+describeRequest(req))
	}
	if a.active != nil {
		return a.broadcastQueue()
	}
	return a.startNext()
}

func describeRequest(req core.RefreshRequest) string {
	text := req.Label()
	switch {
	case req.History:
		text += " (whole history)"
	case len(req.Seasons) > 0:
		text += " (" + strings.Join(req.Seasons, ", ") + ")"
	}
	if req.Why != "" {
		text += " — " + req.Why
	}
	return text
}

// mergeRequest appends req, or merges its seasons into a waiting match.
func mergeRequest(queue []core.RefreshRequest, req core.RefreshRequest) []core.RefreshRequest {
	for i, q := range queue {
		if q.Key() != req.Key() {
			continue
		}
		// The directory, or matches with no years named, asked twice is
		// asked once.
		if req.Directory || (req.TeamID != "" || req.Nations != "") && len(q.Seasons) == 0 && len(req.Seasons) == 0 {
			queue[i] = upgrade(q, req)
			return queue
		}
		// Empty seasons means "current", which can't be merged with years.
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

// upgrade keeps History and user-asked from either request.
func upgrade(req, other core.RefreshRequest) core.RefreshRequest {
	if other.History {
		req.History = true
	}
	if !other.Automatic {
		req.Automatic, req.Why = false, ""
	}
	return req
}

func (a *App) startNext() tea.Cmd {
	if len(a.queue) == 0 {
		a.active = nil
		return a.broadcastQueue()
	}
	if a.active == nil {
		// From idle: a new batch, so the last outcome is no longer news.
		a.err = nil
		a.status = ""
		a.snapshotNews()
	}
	req := a.queue[0]
	a.queue = a.queue[1:]
	a.active = &req
	a.refreshing = a.describe(req, "")
	a.deps.Log.Add(core.LogInfo, "start "+describeRequest(req))
	parent := a.deps.Ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	a.stopActive = cancel
	return tea.Batch(a.broadcastQueue(), runRefresh(ctx, a.deps.Refresh, req))
}

// cancelRefresh drops the queue and stops the running refresh; its final
// event is what returns the app to idle.
func (a *App) cancelRefresh() tea.Cmd {
	if a.active == nil {
		a.status = "nothing is downloading"
		return nil
	}
	a.queue = nil
	a.stopping = true
	a.stopActive()
	a.deps.Log.Add(core.LogInfo, "stop asked: "+a.active.Label()+" and everything queued")
	a.refreshing = "Stopping " + a.active.Label() + "…"
	return a.broadcastQueue()
}

func (a *App) describe(req core.RefreshRequest, progress string) string {
	text := progress
	if text == "" {
		text = "🔄 Refreshing " + req.Label()
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

func (a *App) broadcastQueue() tea.Cmd {
	msg := core.RefreshQueueMsg{Active: a.active, Pending: slices.Clone(a.queue)}
	return a.broadcast(msg)
}

// broadcast reaches the whole stack, so a popped-back screen isn't stale.
func (a *App) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, s := range a.stack {
		updated, cmd := s.Update(msg)
		a.stack[i] = updated
		cmds = append(cmds, a.routeFrom(i, cmd))
	}
	return tea.Batch(cmds...)
}

func (a *App) handleRefreshEvent(msg refreshMsg) tea.Cmd {
	ev := msg.event
	if a.active == nil {
		return nil
	}
	req := *a.active
	if !ev.done {
		a.deps.Log.Add(core.LogProgress, ev.progress)
		if !a.stopping {
			a.refreshing = a.describe(req, ev.progress)
		}
		return awaitRefresh(msg.events)
	}

	a.refreshing = ""
	a.stopActive()
	a.noteRefreshed(req, ev.err)
	switch {
	case a.stopping:
		a.deps.Log.Add(core.LogInfo, "stopped "+req.Label())
	case ev.err != nil:
		a.deps.Log.Add(core.LogError, req.Label()+": "+ev.err.Error())
	default:
		a.deps.Log.Add(core.LogDone, req.Label()+": "+ev.summary)
	}
	if a.stopping {
		// Show what landed before the stop; the cancel error isn't news.
		a.stopping = false
		a.batch = refreshBatch{}
		a.active = nil
		a.status = "Downloads stopped."
		return tea.Batch(a.broadcast(refreshedMsg(req)), a.broadcastQueue())
	}
	if errors.Is(ev.err, core.ErrRateLimited) {
		// The rest would be refused too, so drop it.
		a.queue = nil
		a.batch = refreshBatch{}
		a.status = ""
		a.err = core.ErrRateLimited
		return tea.Batch(a.broadcast(refreshedMsg(req)), a.startNext())
	}
	var reload tea.Cmd
	if ev.err != nil {
		a.batch.failed = append(a.batch.failed, fmt.Sprintf("%s (%v)", req.Label(), ev.err))
		a.batch.lastErr = ev.err
	} else {
		a.batch.done = append(a.batch.done, req.Label())
		if req.PlayerID != "" {
			a.batch.players++
		}
		a.batch.lastSummary = "Refreshed " + req.Label() + ": " + ev.summary
		reload = a.broadcast(refreshedMsg(req))
	}

	// A failure doesn't abandon the rest of the queue.
	var news tea.Cmd
	if len(a.queue) == 0 {
		a.reportBatch()
		news = a.newsSince()
	}
	return tea.Batch(reload, news, a.startNext())
}

func refreshedMsg(req core.RefreshRequest) core.RefreshedMsg {
	return core.RefreshedMsg{
		CompetitionID: req.CompetitionID,
		TeamID:        req.TeamID,
		International: req.TeamID != "" || req.Nations != "" || req.Directory,
		PlayerID:      req.PlayerID,
	}
}

// refreshBatch accumulates outcomes until the queue drains.
type refreshBatch struct {
	done        []string
	players     int // of done, how many were players' stats
	failed      []string
	lastSummary string
	lastErr     error
}

// batchCount reads "3 competitions and 2 players".
func batchCount(comps, players int) string {
	noun := func(n int, word string) string {
		if n == 1 {
			return "1 " + word
		}
		return fmt.Sprintf("%d %ss", n, word)
	}
	switch {
	case players == 0:
		return noun(comps, "competition")
	case comps == 0:
		return noun(players, "player")
	}
	return noun(comps, "competition") + " and " + noun(players, "player")
}

// reportBatch sums up a batch so an early failure isn't hidden by a later
// success.
func (a *App) reportBatch() {
	b := a.batch
	a.batch = refreshBatch{}
	switch {
	case len(b.failed) == 0 && len(b.done) == 1:
		a.status = b.lastSummary
	case len(b.failed) == 0:
		a.status = fmt.Sprintf("Refreshed %s: %s", batchCount(len(b.done)-b.players, b.players), strings.Join(b.done, ", "))
	case len(b.done) == 0 && len(b.failed) == 1:
		a.err = b.lastErr
	default:
		a.err = fmt.Errorf("refreshed %d of %d — failed: %s",
			len(b.done), len(b.done)+len(b.failed), strings.Join(b.failed, "; "))
	}
}
