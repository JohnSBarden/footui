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
// The selected tab is bracketed as well as colored, so the selection
// survives NO_COLOR.
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
		if i == active {
			parts = append(parts, theme.Accent.Bold(true).Render("["+label+"]"))
		} else {
			parts = append(parts, theme.Muted.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, " ")
}
