package dashboard

import (
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// logButton opens the app's log panel; the app handles the click.
func (m *Model) logButton() string {
	th := m.deps.Theme
	label, style := "≡ log", th.Keycap
	if m.logOpen {
		label, style = "× log", th.TabActive.Padding(0, 1)
	}
	return components.Zone("log", style.Render(label))
}
