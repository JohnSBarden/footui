package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// toneMargin is how far a count must be from its expected value before it
// counts as beating or missing it; within it, the difference is noise.
const toneMargin = 0.5

// Tone colours a count by whether it beat its expected value.
func Tone(th styles.Theme, actual, expected float64) lipgloss.Style {
	switch {
	case actual > expected+toneMargin:
		return th.Positive
	case actual < expected-toneMargin:
		return th.Below
	}
	return th.TableCell
}
