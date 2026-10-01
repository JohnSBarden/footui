package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// replyScreen answers a RefreshedMsg with a command, as Browse does when
// it re-runs its search, and records whether the reply came back to it.
type replyScreen struct {
	fakeScreen
	got *[]string
}

type replyMsg struct{ from string }

func (r replyScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case core.RefreshedMsg:
		name := r.name
		return r, func() tea.Msg { return replyMsg{from: name} }
	case replyMsg:
		*r.got = append(*r.got, msg.from)
	}
	return r, nil
}

func TestABuriedScreensReplyComesBackToIt(t *testing.T) {
	browse := replyScreen{fakeScreen: newFake("Browse"), got: new([]string)}
	catalog := replyScreen{fakeScreen: newFake("Download leagues"), got: new([]string)}
	app := newAppWithRefresh(browse, &fakeRefresher{summary: "ok"})
	app, _ = drive(t, app, core.Push(catalog))

	drive(t, app, core.RequestRefresh(core.RefreshRequest{CompetitionID: "esp-la-liga"}))

	if got := *browse.got; len(got) != 1 || got[0] != "Browse" {
		t.Errorf("Browse received replies %v, want its own", got)
	}
	if got := *catalog.got; len(got) != 1 || got[0] != "Download leagues" {
		t.Errorf("the catalog received replies %v, want only its own", got)
	}
}

func TestARepliesForAPoppedScreenAreDropped(t *testing.T) {
	app := newApp(newFake("Browse"))
	app, _ = drive(t, app, core.Push(newFake("League")))
	id := app.ids[1]
	app, _ = drive(t, app, core.Pop())

	if cmd := app.deliver(routedMsg{id: id, msg: replyMsg{}}); cmd != nil {
		t.Error("a reply for a popped screen should go nowhere")
	}
}
