package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// routedMsg carries a message produced by a command that a screen beneath
// the top of the stack issued, addressed back to that screen.
//
// Without it, a buried screen's reply goes to whichever screen is on top.
// That is how Browse used to hang on "Searching…": a refresh finished
// while the catalog was open, Browse re-ran its search, and the results
// were handed to the catalog, which ignored them.
type routedMsg struct {
	id  int
	msg tea.Msg
}

// routeFrom wraps a command issued by the screen at stack index i so its
// message comes back to that screen. The top screen's commands are left
// alone: the normal path already delivers to it, and app-level messages
// it sends (push, pop, status) must still reach the app.
func (a *App) routeFrom(i int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil || i == len(a.stack)-1 {
		return cmd
	}
	return route(a.ids[i], cmd)
}

func route(id int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		switch m := msg.(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			wrapped := make(tea.BatchMsg, 0, len(m))
			for _, c := range m {
				wrapped = append(wrapped, route(id, c))
			}
			return wrapped
		}
		return routedMsg{id: id, msg: msg}
	}
}

// deliver hands a routed message to its screen if it is still on the
// stack. App-level messages pass through to the app; a reply for a screen
// that has since been popped is dropped.
func (a *App) deliver(r routedMsg) tea.Cmd {
	if isAppMsg(r.msg) {
		_, cmd := a.Update(r.msg)
		return cmd
	}
	for i, id := range a.ids {
		if id != r.id {
			continue
		}
		if i == len(a.stack)-1 {
			_, cmd := a.Update(r.msg)
			return cmd
		}
		updated, cmd := a.stack[i].Update(r.msg)
		a.stack[i] = updated
		return a.routeFrom(i, cmd)
	}
	return nil
}

// isAppMsg reports whether msg is addressed to the app rather than to a
// screen.
func isAppMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case core.PushMsg, core.PopMsg, core.QuitMsg, core.StatusMsg, core.RefreshRequestMsg,
		core.FavoriteToggledMsg, tea.QuitMsg:
		return true
	}
	return false
}
