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
	// splashFor is how long the launch splash stays when nothing makes
	// it wait.
	splashFor = 2 * time.Second
	// splashFrameEvery paces the splash's animation.
	splashFrameEvery = 50 * time.Millisecond
	// splashSettleAfter is how long the balls bounce at full height
	// before, with nothing left to refresh, they come to rest; settling
	// takes about 0.9 s, so on an idle launch they stop as it closes.
	splashSettleAfter = 1100 * time.Millisecond
	splashSpinEvery   = 80 * time.Millisecond
	splashListWidth   = 50
	splashBarWidth    = 44
)

var splashSpinner = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// splashDoneMsg is the splash's timer running out.
type splashDoneMsg struct{}

// splashFrameMsg asks for the splash's next animation frame.
type splashFrameMsg struct{}

// splashState is the launch screen: footui's name with its two bouncing
// balls, what the launch is bringing up to date, and whether a provider
// has paused footui. It never blocks: any key skips it, and it goes on
// its own.
type splashState struct {
	// waitForData keeps it up on an empty database until the first
	// refresh lands, since the screen beneath has nothing to show yet.
	waitForData bool
	// done are the refreshes that finished while it was showing.
	done []splashDone
	// elapsed is how long it has been animating, counted in frames so
	// the animation doesn't depend on the clock.
	elapsed time.Duration
	balls   splashBalls
}

// splashDone is one finished refresh, and whether it failed.
type splashDone struct {
	label  string
	failed bool
}

// ShowSplash puts the splash in front of the first screen.
func (a *App) ShowSplash(waitForData bool) {
	a.splash = &splashState{waitForData: waitForData, balls: newSplashBalls()}
}

func splashTimer() tea.Cmd {
	return tea.Tick(splashFor, func(time.Time) tea.Msg { return splashDoneMsg{} })
}

func splashFrame() tea.Cmd {
	return tea.Tick(splashFrameEvery, func(time.Time) tea.Msg { return splashFrameMsg{} })
}

// splashAnimate moves the splash on a frame and asks for the next, until
// the splash has gone. The balls settle once they've had their bounce and
// nothing is left to refresh.
func (a *App) splashAnimate() tea.Cmd {
	if a.splash == nil {
		return nil
	}
	s := a.splash
	s.elapsed += splashFrameEvery
	settling := s.elapsed >= splashSettleAfter && a.active == nil && len(a.queue) == 0
	s.balls.step(splashFrameEvery.Seconds(), settling)
	return splashFrame()
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
	body := a.splashBody(th)
	// The key hint takes the last row; the block is centred above it.
	room := a.height - 1
	head := components.Wordmark(th)
	if a.width >= artWidth+2 && room >= artRows+lipgloss.Height(body) {
		head = splashArt(th, a.splash.balls)
	}
	block := lipgloss.JoinVertical(lipgloss.Center, head, body)
	hint := " " + th.Keycap.Render("any key") + " " + th.Help.Render("skip")
	return lipgloss.Place(a.width, room, lipgloss.Center, lipgloss.Center, block) + "\n" + hint
}

// splashBody is everything under the wordmark: the tagline, then what the
// launch is refreshing, a progress bar and where it has got to, then any
// provider pause. Rows are padded to one width so they line up when the
// block is centred.
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
			lines = append(lines, splashTask(width, th.Faint.Render("·"), th.Muted, q.Label(), ""))
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

// splashTagline is the line under the wordmark, with the build's version
// when there is one.
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

// splashTask is one refresh on the splash: its icon, its label, and on
// the right what has become of it.
func splashTask(width int, icon string, label lipgloss.Style, text, meta string) string {
	room := max(1, width-2-lipgloss.Width(meta)-1)
	return padBetween(icon+" "+label.Render(components.Truncate(text, room)), meta, width)
}

// padBetween puts left and right at either end of width columns.
func padBetween(left, right string, width int) string {
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}
