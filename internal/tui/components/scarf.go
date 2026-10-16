package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// scarfRun is how many cells each colour of a scarf stripe spans.
const scarfRun = 3

// ScarfTop is a card's top border striped in a club's two colours, width
// cells wide including the corners. It replaces the border's own top edge.
func ScarfTop(th styles.Theme, primaryHex, secondaryHex string, width int) string {
	if width < 2 {
		return strings.Repeat("─", max(width, 0))
	}
	inner := width - 2
	border := lipgloss.NewStyle().Foreground(th.Palette.Border)
	a, okA := styles.ClubForeground(primaryHex)
	if th.NoColor || !okA {
		if th.NoColor {
			border = lipgloss.NewStyle()
		}
		return border.Render("╭" + strings.Repeat("─", inner) + "╮")
	}
	b, okB := styles.ClubForeground(secondaryHex)
	if !okB {
		b = a
	}
	var s strings.Builder
	for i := 0; i < inner; i += scarfRun {
		n := min(scarfRun, inner-i)
		c := a
		if (i/scarfRun)%2 == 1 {
			c = b
		}
		s.WriteString(lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("━", n)))
	}
	return border.Render("╭") + s.String() + border.Render("╮")
}
