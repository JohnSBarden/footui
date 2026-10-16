package components

import (
	"strings"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Label is a section heading: upper case, muted, bold.
func Label(th styles.Theme, s string) string {
	return th.Label.Render(strings.ToUpper(s))
}

// Wordmark is footui's name as the app's title chip.
func Wordmark(th styles.Theme) string {
	return th.Title.Render("● footui")
}
