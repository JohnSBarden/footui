package components

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFlagsComeFromTheNationsID(t *testing.T) {
	for id, want := range map[string]string{
		"nat-france":              "🇫🇷",
		"nat-united-states-women": "🇺🇸",
		"nat-usa-u23":             "🇺🇸",
		"nat-republic-of-ireland": "🇮🇪",
		"nat-england":             "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F",
		"nat-basque-country":      "",
		"nat-arcadia":             "",
	} {
		if got := Flag(id); got != want {
			t.Errorf("Flag(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestTrophyAndGoalNetAreTwoCellsWide(t *testing.T) {
	for _, e := range []string{"🏆", "🥅"} {
		if w := lipgloss.Width(e); w != 2 {
			t.Errorf("%s is %d cells", e, w)
		}
	}
}

func TestEmojiIsOffOnTheLinuxConsole(t *testing.T) {
	for term, want := range map[string]bool{"xterm-ghostty": true, "xterm-256color": true, "": true, "linux": false, "dumb": false, "vt100": false} {
		if got := emojiTerminal(term); got != want {
			t.Errorf("emojiTerminal(%q) = %v", term, got)
		}
	}
}

func TestAFlagIsTwoCellsWide(t *testing.T) {
	for _, id := range []string{"nat-japan", "nat-scotland"} {
		if w := lipgloss.Width(Flag(id)); w != 2 {
			t.Errorf("%s's flag is %d cells", id, w)
		}
	}
}
