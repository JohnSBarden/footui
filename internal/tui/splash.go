package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

const (
	splashFor        = 2 * time.Second
	splashFrameEvery = 50 * time.Millisecond
	// splashSettleAfter plus ~0.9 s of settling ends as splashFor does.
	splashSettleAfter = 1100 * time.Millisecond
	splashSpinEvery   = 80 * time.Millisecond
	// splashHold keeps "ready" on screen for a moment once the startup
	// refreshes finish after splashFor, so the last ✓ is seen.
	splashHold      = 700 * time.Millisecond
	splashListWidth = 50
	splashBarWidth  = 44
)

var splashSpinner = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

type splashDoneMsg struct{}

type splashFrameMsg struct{}

// splashState is the launch screen. It stays at least splashFor and then
// until the startup refreshes finish, but never blocks: any key skips it.
type splashState struct {
	// waitForData ends it on an empty database as soon as the first
	// refresh lands, rather than after the whole queue.
	waitForData bool
	done        []splashDone
	// elapsed is counted in frames, so it doesn't depend on the clock.
	elapsed time.Duration
	balls   splashBalls
	// timedOut: splashFor has passed, so it goes as soon as it's idle.
	timedOut bool
	// idleSince is the elapsed time the queue last emptied, -1 while busy.
	idleSince time.Duration
}

type splashDone struct {
	label  string
	failed bool
}

// ShowSplash puts the splash in front of the first screen.
func (a *App) ShowSplash(waitForData bool) {
	a.splash = &splashState{waitForData: waitForData, balls: newSplashBalls(), idleSince: -1}
}

// splashBusy: a startup refresh is running or waiting.
func (a *App) splashBusy() bool {
	return a.active != nil || len(a.queue) > 0
}

func splashTimer() tea.Cmd {
	return tea.Tick(splashFor, func(time.Time) tea.Msg { return splashDoneMsg{} })
}

func splashFrame() tea.Cmd {
	return tea.Tick(splashFrameEvery, func(time.Time) tea.Msg { return splashFrameMsg{} })
}

// splashAnimate settles the balls once they've bounced and nothing is left
// to refresh, and ends the splash once the refreshes that outlasted
// splashFor are done and "ready" has been up for splashHold.
func (a *App) splashAnimate() tea.Cmd {
	if a.splash == nil {
		return nil
	}
	s := a.splash
	s.elapsed += splashFrameEvery
	busy := a.splashBusy()
	switch {
	case busy:
		s.idleSince = -1
	case s.idleSince < 0:
		s.idleSince = s.elapsed
	}
	if s.timedOut && !busy && s.elapsed-s.idleSince >= splashHold {
		a.splash = nil
		return nil
	}
	settling := s.elapsed >= splashSettleAfter && !busy
	s.balls.step(splashFrameEvery.Seconds(), settling)
	return splashFrame()
}

// splashTimedOut ends the splash if nothing is refreshing; otherwise it
// stays until the queue is done (see splashAnimate).
func (a *App) splashTimedOut() {
	if a.splash == nil {
		return
	}
	a.splash.timedOut = true
	if !a.splashBusy() {
		a.splash = nil
	}
}

// noteRefreshed: the first success on an empty database ends the splash.
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

const splashKofi = "☕ ko-fi.com/johnnycashmoney"

func (a *App) splashView() string {
	th := a.theme()
	body := a.splashBody(th)
	// The key hint takes the last row; the block is centred above it.
	room := a.height - 1
	head := components.Wordmark(th)
	if a.width >= artWidth+2 && room >= artRows+lipgloss.Height(body) {
		head = splashArt(th, a.splash.balls)
	}
	block := lipgloss.JoinVertical(lipgloss.Center, head, body)
	hint := " " + th.Keycap.Render("any key") + " " + th.Help.Render("skip")
	if kofi := th.Faint.Render(splashKofi + " "); a.width >= lipgloss.Width(hint)+lipgloss.Width(kofi)+2 {
		hint = padBetween(hint, kofi, a.width)
	}
	return lipgloss.Place(a.width, room, lipgloss.Center, lipgloss.Center, block) + "\n" + hint
}

// splashBody pads rows to one width so they line up when centred.
func (a *App) splashBody(th styles.Theme) string {
	lines := []string{"", th.Muted.Render(a.splashTagline())}
	width := min(splashListWidth, a.width-2)
	done := a.splash.done
	n := len(a.queue) + len(done)
	if a.active != nil {
		n++
	}
	if n > 0 {
		lines = append(lines, "", "")
		failed := 0
		for _, d := range done {
			if d.failed {
				failed++
				lines = append(lines, splashTask(width, th.Negative.Render("✕"), th.TableCell, d.label, th.Negative.Render("failed")))
			} else {
				lines = append(lines, splashTask(width, th.Positive.Render("✓"), th.TableCell, d.label, th.Muted.Render("up to date")))
			}
		}
		if a.active != nil {
			spin := splashSpinner[int(a.splash.elapsed/splashSpinEvery)%len(splashSpinner)]
			lines = append(lines, splashTask(width, th.Accent.Render(string(spin)), th.TableCell, a.active.Label(), th.Muted.Render("refreshing")))
		}
		for _, q := range a.queue {
			lines = append(lines, splashTask(width, th.Faint.Render("·"), th.Muted, q.Label(), th.Faint.Render(q.Why)))
		}

		// The refresh under way counts as half done.
		progress := float64(len(done))
		if a.active != nil {
			progress += 0.5
		}
		progress /= float64(n)
		finished := a.active == nil && len(a.queue) == 0
		bar := max(0, min(splashBarWidth, width-6))
		filled := int(math.Round(progress * float64(bar)))
		pct := th.TableCell
		if finished {
			pct = th.Positive
		}
		meter := th.Accent.Render(strings.Repeat("█", filled)) + th.Faint.Render(strings.Repeat("░", bar-filled))
		lines = append(lines, "", padBetween(meter, pct.Bold(true).Render(fmt.Sprintf("%d%%", int(progress*100))), width))

		status := th.Muted.Render("syncing local cache…")
		switch {
		case finished && failed > 0:
			status = th.Negative.Render(fmt.Sprintf("ready · %d failed", failed))
		case finished:
			status = th.Positive.Render("ready · cache up to date")
		}
		lines = append(lines, padBetween(status, "", width))
	}
	if a.deps.ProviderPause != nil {
		if until := a.deps.ProviderPause(); !until.IsZero() && time.Now().Before(until) {
			mins := int(time.Until(until).Minutes()) + 1
			lines = append(lines, "", th.Negative.Render(fmt.Sprintf("✕ refreshes paused: retrying in %d min", mins)))
		}
	}
	return strings.Join(lines, "\n")
}

func (a *App) splashTagline() string {
	const tagline = "football stats in your terminal"
	v := a.deps.Version
	if v == "" {
		return tagline
	}
	// A bare release number gets its v; a commit hash doesn't.
	if v[0] >= '0' && v[0] <= '9' && strings.Contains(v, ".") {
		v = "v" + v
	}
	return tagline + " · " + v
}

func splashTask(width int, icon string, label lipgloss.Style, text, meta string) string {
	room := max(1, width-2-lipgloss.Width(meta)-1)
	return padBetween(icon+" "+label.Render(components.Truncate(text, room)), meta, width)
}

// padBetween puts left and right at either end of width columns.
func padBetween(left, right string, width int) string {
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}
