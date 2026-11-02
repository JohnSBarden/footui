// Package logpanel draws the activity log beside any screen: what's
// downloading, what's queued, and recent entries newest first.
package logpanel

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/trace"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Geometry: the panel's width beside a screen at least wideFrom or
// besideFrom wide, and the queued requests listed before "… n more".
const (
	narrow     = 36
	wide       = 46
	besideFrom = 80
	wideFrom   = 120
	// Gap is the space between the screen and the panel.
	Gap    = 2
	queued = 4
)

// Width is the panel's width in a terminal total wide; the whole width
// when there's no room for a screen beside it.
func Width(total int) int {
	switch {
	case total >= wideFrom:
		return wide
	case total >= besideFrom:
		return narrow
	}
	return total
}

// Panel is what the panel shows.
type Panel struct {
	Theme styles.Theme
	Log   *core.Log
	Queue core.RefreshQueueMsg
	// Pause is when a rate-limit pause ends; nil means no pauses.
	Pause func() time.Time
	Now   time.Time
}

// View renders the panel at width × height.
func (p Panel) View(width, height int) string {
	th := p.Theme
	inner := max(width-4, 8)
	fit := func(s string) string { return components.Truncate(s, inner) }

	subtitle := " · downloads"
	if trace.On() {
		subtitle = " · debug"
	}
	lines := []string{th.PanelTitle.Render(fit("Log")) + th.Muted.Render(subtitle)}
	if path := trace.Path(); path != "" {
		lines = append(lines, th.Faint.Render("→ "+tail(tilde(path), inner-2)))
	}
	lines = append(lines, th.Label.Render("NOW"))
	entries := p.Log.Entries()
	if a := p.Queue.Active; a != nil {
		lines = append(lines, th.Status.Render(fit("🔄 "+a.Label())))
		if pr := latestProgress(entries); pr != "" {
			lines = append(lines, th.Muted.Render(fit("  "+pr)))
		}
	} else {
		lines = append(lines, th.Muted.Render(fit("idle — nothing downloading")))
	}
	if p.Pause != nil {
		if until := p.Pause(); until.After(p.Now) {
			lines = append(lines, th.Negative.Render(fit("⏸ paused until "+until.Format("15:04"))))
		}
	}
	if n := len(p.Queue.Pending); n > 0 {
		lines = append(lines, th.Label.Render("QUEUED "+strconv.Itoa(n)))
		for i, r := range p.Queue.Pending {
			if i == queued && n > queued+1 {
				lines = append(lines, th.Faint.Render(fit("  … "+strconv.Itoa(n-i)+" more")))
				break
			}
			lines = append(lines, th.TableCell.Render(fit("  · "+r.Label())))
		}
	}

	lines = append(lines, th.Label.Render("RECENT"))
	// The panel's border takes two lines.
	room := height - 2 - len(lines)
	if len(entries) == 0 {
		lines = append(lines, th.Faint.Render(fit("nothing yet this session")))
	}
	for i := len(entries) - 1; i >= 0 && room > 0; i-- {
		lines = append(lines, p.line(entries[i], inner))
		room--
	}
	if len(lines) > height-2 {
		lines = lines[:max(height-2, 1)]
	}
	return th.Panel.Width(width - 2).Height(height - 2).BorderForeground(th.Palette.Border).Render(strings.Join(lines, "\n"))
}

func (p Panel) line(e core.LogEntry, inner int) string {
	th := p.Theme
	mark, style := "·", th.Muted
	switch e.Level {
	case core.LogProgress:
		mark, style = "…", th.Faint
	case core.LogDone:
		mark, style = "✓", th.Positive
	case core.LogError:
		mark, style = "✗", th.Negative
	case core.LogDebug:
		mark, style = "›", th.Faint
	}
	stamp := e.At.Format("15:04:05") + " "
	room := max(inner-len(stamp), 1)
	if e.Level == core.LogDebug {
		return th.Faint.Render(stamp) + style.Render(mark+" "+tail(dropHost(e.Text), room-2))
	}
	return th.Faint.Render(stamp) + style.Render(components.Truncate(mark+" "+e.Text, room))
}

var hostPrefix = regexp.MustCompile(`https?://[^/\s]+`)

// dropHost shortens a request line to its path: the end, with the status
// and time, is what's worth the room.
func dropHost(s string) string { return hostPrefix.ReplaceAllString(s, "") }

// tail keeps the last width cells of s, marking the cut with "…".
func tail(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[1:]
	}
	return "…" + string(r)
}

// latestProgress only looks back to the last start entry.
func latestProgress(entries []core.LogEntry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		switch e := entries[i]; e.Level {
		case core.LogProgress:
			return e.Text
		case core.LogInfo:
			if strings.HasPrefix(e.Text, "start ") {
				return ""
			}
		}
	}
	return ""
}

func tilde(path string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

// Beside joins screen and panel side by side, screen padded to
// screenWidth.
func Beside(screen, panel string, screenWidth int) string {
	left := strings.Split(screen, "\n")
	right := strings.Split(panel, "\n")
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = l + strings.Repeat(" ", max(screenWidth-lipgloss.Width(l), 0)+Gap) + r
	}
	return strings.Join(out, "\n")
}
