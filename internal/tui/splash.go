package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// splashFor is how long the launch splash stays when nothing makes it wait.
const splashFor = 1500 * time.Millisecond

// splashDoneMsg is the splash's timer running out.
type splashDoneMsg struct{}

// splashState is the launch screen: footui's name, what the launch is
// bringing up to date, and whether a provider has paused footui. It never
// blocks: any key skips it, and it goes on its own.
type splashState struct {
	// waitForData keeps it up on an empty database until the first
	// refresh lands, since the screen beneath has nothing to show yet.
	waitForData bool
	// done are the refreshes that finished while it was showing.
	done []splashDone
}

// splashDone is one finished refresh, and whether it failed.
type splashDone struct {
	label  string
	failed bool
}

// ShowSplash puts the splash in front of the first screen.
func (a *App) ShowSplash(waitForData bool) {
	a.splash = &splashState{waitForData: waitForData}
}

func splashTimer() tea.Cmd {
	return tea.Tick(splashFor, func(time.Time) tea.Msg { return splashDoneMsg{} })
}

// splashTimedOut ends the splash unless it is waiting for a first league
// that is actually on its way.
func (a *App) splashTimedOut() {
	if a.splash == nil {
		return
	}
	coming := a.active != nil || len(a.queue) > 0
	if !a.splash.waitForData || !coming {
		a.splash = nil
	}
}

// noteRefreshed records a finished refresh on the splash; the first one to
// land on an empty database is what the splash was waiting for.
func (a *App) noteRefreshed(req core.RefreshRequest, err error) {
	if a.splash == nil {
		return
	}
	a.splash.done = append(a.splash.done, splashDone{label: req.Label(), failed: err != nil})
	if err != nil {
		return
	}
	if a.splash.waitForData {
		a.splash = nil
	}
}

func (a *App) splashView() string {
	th := a.theme()
	lines := []string{components.Wordmark(th)}
	n := len(a.queue) + len(a.splash.done)
	if a.active != nil {
		n++
	}
	if n > 0 {
		noun := "leagues"
		if n == 1 {
			noun = "league"
		}
		lines = append(lines, "", th.TableCell.Render(fmt.Sprintf("Refreshing %d %s…", n, noun)))
		for _, d := range a.splash.done {
			if d.failed {
				lines = append(lines, th.Negative.Render("✕ ")+th.TableCell.Render(d.label))
			} else {
				lines = append(lines, th.Positive.Render("✓ ")+th.TableCell.Render(d.label))
			}
		}
		if a.active != nil {
			lines = append(lines, th.Accent.Render("◐ ")+th.TableCell.Render(a.active.Label()))
		}
		for _, q := range a.queue {
			lines = append(lines, th.Faint.Render("◌ ")+th.Muted.Render(q.Label()))
		}
	}
	if a.deps.ProviderPause != nil {
		if name, until := a.deps.ProviderPause(); !until.IsZero() && time.Now().Before(until) {
			mins := int(time.Until(until).Minutes()) + 1
			lines = append(lines, "", th.Negative.Render(fmt.Sprintf("✕ %s paused requests: retrying in %d min", name, mins)))
		}
	}
	// Left-aligned as a block, the block centred on the screen.
	block := lipgloss.NewStyle().Render(strings.Join(lines, "\n"))
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, block)
}
