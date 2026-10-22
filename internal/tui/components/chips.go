package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// chipText is the dark text on a weak-theme result chip. Strong-theme
// result colours can be dark, so there the text is solved per chip.
const chipText = "#0b0b0b"

// FormChips draws a form string ("WDLWW", oldest first) as letter chips.
func FormChips(th styles.Theme, form string) string {
	if form == "" {
		return ""
	}
	parts := make([]string, 0, len(form))
	for i := 0; i < len(form); i++ {
		r := form[i]
		if th.NoColor {
			parts = append(parts, string(r))
			continue
		}
		parts = append(parts, chipStyle(th, r).Render(" "+string(r)+" "))
	}
	if th.NoColor {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts, "")
}

func chipStyle(th styles.Theme, r byte) lipgloss.Style {
	bg := th.Palette.Draw
	switch r {
	case 'W':
		bg = th.Palette.Positive
	case 'L':
		bg = th.Palette.Negative
	}
	return lipgloss.NewStyle().Bold(true).Foreground(styles.TextOn(bg, lipgloss.Color(chipText))).Background(bg)
}

// ResultGlyph is one game's result as a shape and colour: ● won, ◐ drawn,
// ○ lost. The shapes differ, so it reads without colour too.
func ResultGlyph(th styles.Theme, r byte) string {
	switch r {
	case 'W':
		return th.Positive.Render("●")
	case 'L':
		return th.Negative.Render("○")
	case 'D':
		return th.Draw.Render("◐")
	}
	return " "
}
