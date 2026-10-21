package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Tab is one entry in a tab bar.
type Tab struct {
	Label string
	// Badge is optional trailing detail, e.g. a row count.
	Badge string
}

// Tabs renders a horizontal tab bar with the active index highlighted.
// Each tab is clickable as "tab:"+its index.
func Tabs(theme styles.Theme, tabs []Tab, active int) string {
	if len(tabs) == 0 {
		return ""
	}

	parts := make([]string, 0, len(tabs))
	for i, t := range tabs {
		label := t.Label
		if t.Badge != "" {
			label += " " + t.Badge
		}
		parts = append(parts, Zone("tab:"+itoa(i), TabLabel(theme, label, i == active)))
	}
	return strings.Join(parts, " ")
}

// TabLabel is one tab: a filled pill when active, a muted one otherwise.
// Without colour the active tab is bracketed, so the selection survives
// NO_COLOR.
func TabLabel(th styles.Theme, label string, active bool) string {
	if th.NoColor {
		if active {
			return th.Accent.Bold(true).Render("[" + label + "]")
		}
		return th.Muted.Render(" " + label + " ")
	}
	if active {
		return th.TabActive.Render(" " + label + " ")
	}
	return th.TabIdle.Render(" " + label + " ")
}

// WindowTabs joins tabs, ordered left to right, into a bar at most
// maxWidth wide. When they don't all fit, it shows a window around the
// selected one, growing towards the right first, with ‹ and › marking the
// tabs off either end, so the selection is never the one scrolled away.
func WindowTabs(th styles.Theme, tabs []string, sel, maxWidth int) string {
	n := len(tabs)
	if n == 0 {
		return ""
	}
	sel = min(max(sel, 0), n-1)
	const marker = 2
	width := func(lo, hi int) int {
		w := 0
		for d := lo; d <= hi; d++ {
			w += lipgloss.Width(tabs[d]) + 1
		}
		if lo > 0 {
			w += marker
		}
		if hi < n-1 {
			w += marker
		}
		return w - 1
	}
	lo, hi := sel, sel
	for grew := true; grew; {
		grew = false
		if hi < n-1 && width(lo, hi+1) <= maxWidth {
			hi++
			grew = true
		}
		if lo > 0 && width(lo-1, hi) <= maxWidth {
			lo--
			grew = true
		}
	}

	var b strings.Builder
	if lo > 0 {
		b.WriteString(th.Muted.Render("‹ "))
	}
	b.WriteString(strings.Join(tabs[lo:hi+1], " "))
	if hi < n-1 {
		b.WriteString(th.Muted.Render(" ›"))
	}
	return b.String()
}
