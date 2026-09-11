package tui

import (
	"context"
	"fmt"

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

// runRefresh starts refresh for competitionID and returns its first event.
// The goroutine is started inside the command rather than in Update so a
// test can call the command synchronously and drive the chain by hand.
func runRefresh(ctx context.Context, refresh core.Refresher, competitionID string) tea.Cmd {
	return func() tea.Msg {
		events := make(chan refreshEvent, 16)
		go func() {
			defer close(events)
			summary, err := refresh(ctx, competitionID, func(text string) {
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
	if entry, ok := a.current().(core.TextEntry); ok && entry.CapturingText() {
		return keys.RefreshFromTextEntry, true
	}
	return keys.Refresh, true
}

// startRefresh handles the refresh key. Every way it can decline is
// explained in the status line: the key is advertised in the footer, so
// pressing it and seeing nothing happen would be a bug from the user's
// side of the screen.
func (a *App) startRefresh() tea.Cmd {
	if a.refreshing != "" {
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
	a.err = nil
	a.lastRefreshTarget = compID
	a.refreshing = "Refreshing " + compID + "…"
	return runRefresh(a.deps.Ctx, a.deps.Refresh, compID)
}

// handleRefreshEvent applies one event from the running refresh.
func (a *App) handleRefreshEvent(msg refreshMsg) tea.Cmd {
	ev := msg.event
	if !ev.done {
		a.refreshing = ev.progress
		return awaitRefresh(msg.events)
	}

	a.refreshing = ""
	if ev.err != nil {
		a.err = ev.err
		return nil
	}

	a.status = "Refreshed " + a.lastRefreshTarget + ": " + ev.summary
	// Every screen on the stack is told, not just the visible one, so a
	// screen the user pops back to isn't still showing its empty state.
	var cmds []tea.Cmd
	for i, s := range a.stack {
		updated, cmd := s.Update(core.RefreshedMsg{CompetitionID: a.lastRefreshTarget})
		a.stack[i] = updated
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}
