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

// snapshotNews is taken as a batch starts, for newsSince to diff against.
func (a *App) snapshotNews() {
	if a.deps.Repos == nil {
		return
	}
	a.newsBefore = ticker.Take(a.deps.Ctx, a.deps.Repos, a.now())
}

// newsSince puts the batch's news on the ticker, led by the batch summary
// it displaces from the status line.
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
