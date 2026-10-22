package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// routedMsg addresses a buried screen's command result back to it, rather
// than to whichever screen is on top.
type routedMsg struct {
	id  int
	msg tea.Msg
}

// routeFrom leaves the top screen's commands alone, so its app-level
// messages (push, pop, status) still reach the app.
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

// deliver drops a reply for a screen that has since been popped;
// app-level messages pass through.
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

func isAppMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case core.PushMsg, core.PopMsg, core.StatusMsg, core.RefreshRequestMsg,
		core.FavoriteToggledMsg, tea.QuitMsg:
		return true
	}
	return false
}
