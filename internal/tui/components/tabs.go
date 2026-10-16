package components

import (
	"strings"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Tab is one entry in a tab bar.
type Tab struct {
	Label string
	// Badge is optional trailing detail, e.g. a row count.
	Badge string
}

// Tabs renders a horizontal tab bar with the active index highlighted.
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
		parts = append(parts, TabLabel(theme, label, i == active))
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
