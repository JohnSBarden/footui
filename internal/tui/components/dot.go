package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Dot marks a club by its primary colour; a club without one gets a faint
// dot so names still line up.
func Dot(th styles.Theme, primaryHex string) string {
	if th.NoColor {
		return "●"
	}
	if c, ok := styles.ClubForeground(primaryHex); ok {
		return lipgloss.NewStyle().Foreground(c).Render("●")
	}
	return th.Faint.Render("●")
}
