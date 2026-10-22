package dashboard

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// Log panel geometry: how wide it is beside the cards (wider on a wide
// terminal), and the most of what's queued it lists by name.
const (
	logWidth     = 36
	logWideWidth = 46
	logQueued    = 4
)

// logPanelWidth is how wide the log panel is, zero while it's closed.
// On a terminal too narrow for it and a column of cards, it takes the
// whole width.
func (m *Model) logPanelWidth() int {
	switch {
	case !m.showLog:
		return 0
	case m.width >= 3*minCardWidth+logWideWidth:
		return logWideWidth
	case m.width >= minCardWidth+cardGap+logWidth:
		return logWidth
	}
	return m.width
}

// cardsWidth is the width the cards flow into: the screen's, less the log
// panel's when it's open.
func (m *Model) cardsWidth() int {
	if w := m.logPanelWidth(); w > 0 {
		return max(m.width-w-cardGap, 0)
	}
	return m.width
}

// logButton is the header's log button, a click target; it reads as
// pressed while the panel is open.
func (m *Model) logButton() string {
	th := m.deps.Theme
	label, style := "≡ log", th.Keycap
	if m.showLog {
		label, style = "× log", th.TabActive.Padding(0, 1)
	}
	return components.Zone("log", style.Render(label))
}

// withLog puts the log panel to the right of the cards, height lines
// tall; on a narrow terminal it replaces them.
func (m *Model) withLog(cards string, height int) string {
	w := m.logPanelWidth()
	panel := m.logPanel(w, height)
	if w >= m.width {
		return panel
	}
	cw := m.cardsWidth()
	left := strings.Split(cards, "\n")
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
		out[i] = strings.TrimRight(l+strings.Repeat(" ", max(cw-lipgloss.Width(l), 0)+cardGap)+r, " ")
	}
	return strings.Join(out, "\n")
}

// logPanel is what's downloading now, what's waiting, and the log of
// what happened, newest first, as much as fits in height lines.
func (m *Model) logPanel(width, height int) string {
	th := m.deps.Theme
	inner := max(width-4, 8)
	fit := func(s string) string { return components.Truncate(s, inner) }

	lines := []string{th.PanelTitle.Render(fit("Log")) + th.Muted.Render(" · downloads")}
	lines = append(lines, th.Label.Render("NOW"))
	entries := m.deps.Log.Entries()
	if a := m.queue.Active; a != nil {
		lines = append(lines, th.Status.Render(fit("🔄 "+a.Label())))
		if p := latestProgress(entries); p != "" {
			lines = append(lines, th.Muted.Render(fit("  "+p)))
		}
	} else {
		lines = append(lines, th.Muted.Render(fit("idle — nothing downloading")))
	}
	if pause := m.deps.ProviderPause; pause != nil {
		if until := pause(); until.After(now()) {
			lines = append(lines, th.Negative.Render(fit("⏸ paused until "+until.Format("15:04"))))
		}
	}
	if n := len(m.queue.Pending); n > 0 {
		lines = append(lines, th.Label.Render("QUEUED "+strconv.Itoa(n)))
		for i, r := range m.queue.Pending {
			if i == logQueued && n > logQueued+1 {
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
		lines = append(lines, m.logLine(entries[i], inner))
		room--
	}
	if len(lines) > height-2 {
		lines = lines[:max(height-2, 1)]
	}
	return th.Panel.Width(width - 2).Height(height - 2).BorderForeground(th.Palette.Border).Render(strings.Join(lines, "\n"))
}

// logLine is one entry: its time, a mark for how it went, and its text.
func (m *Model) logLine(e core.LogEntry, inner int) string {
	th := m.deps.Theme
	mark, style := "·", th.Muted
	switch e.Level {
	case core.LogProgress:
		mark, style = "…", th.Faint
	case core.LogDone:
		mark, style = "✓", th.Positive
	case core.LogError:
		mark, style = "✗", th.Negative
	}
	stamp := e.At.Format("15:04:05") + " "
	return th.Faint.Render(stamp) + style.Render(components.Truncate(mark+" "+e.Text, max(inner-len(stamp), 1)))
}

// latestProgress is the running download's last progress line: the
// newest one since the last thing started.
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
