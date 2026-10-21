package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/ticker"
)

// tickerMsg moves the news ticker on to its next item.
type tickerMsg struct{}

func tickNews() tea.Cmd {
	return tea.Tick(ticker.Every, func(time.Time) tea.Msg { return tickerMsg{} })
}

// snapshotNews remembers the followed teams as a batch of refreshes
// starts, for newsSince to compare against once it drains.
func (a *App) snapshotNews() {
	if a.deps.Repos == nil {
		return
	}
	a.newsBefore = ticker.Take(a.deps.Ctx, a.deps.Repos, a.now())
}

// newsSince puts what the batch brought in for the followed teams on the
// ticker, led by the batch's own summary, which would otherwise sit on
// the status line in its place. It starts the ticker turning if it
// wasn't.
func (a *App) newsSince() tea.Cmd {
	if a.deps.Repos == nil {
		return nil
	}
	now := a.now()
	after := ticker.Take(a.deps.Ctx, a.deps.Repos, now)
	news := ticker.News(a.deps.Ctx, a.deps.Repos, a.newsBefore, after)
	a.newsBefore = ticker.Snapshot{}
	if len(news) == 0 {
		return nil
	}
	if a.status != "" {
		news = append([]string{a.status}, news...)
		a.status = ""
	}
	a.news.Add(now, news...)
	if a.newsTurning {
		return nil
	}
	a.newsTurning = true
	return tickNews()
}

// turnNews shows the ticker's next item, and stops turning once it has
// nothing left.
func (a *App) turnNews() tea.Cmd {
	if !a.news.Advance(a.now()) {
		a.newsTurning = false
		return nil
	}
	return tickNews()
}

func (a *App) now() time.Time {
	if a.clock != nil {
		return a.clock()
	}
	return time.Now()
}
