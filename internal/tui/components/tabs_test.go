package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestTabsKeepBracketsWithoutColor(t *testing.T) {
	got := Tabs(styles.Plain(), []Tab{{Label: "Premier League"}, {Label: "Champions League"}}, 0)
	if !strings.Contains(got, "[Premier League]") {
		t.Errorf("plain tabs = %q", got)
	}
}

func TestTabsArePillsInColor(t *testing.T) {
	got := ansi.Strip(Tabs(styles.Default(), []Tab{{Label: "A"}, {Label: "B"}}, 1))
	if got != " A   B " {
		t.Errorf("pills = %q", got)
	}
}

func TestStatCardLabelsAreUpperCase(t *testing.T) {
	got := ansi.Strip(StatCards(styles.Default(), []Stat{{Label: "Points", Value: "6"}}, 80))
	if !strings.Contains(got, "POINTS") {
		t.Errorf("card = %q", got)
	}
}

// A card without a hint keeps its third line when a neighbour has one, so
// the row's bottom borders line up.
func TestStatCardsInARowShareAHeight(t *testing.T) {
	stats := []Stat{{Label: "Goals", Value: "8:3", Hint: "+5"}, {Label: "Possession", Value: "57%"}}
	got := ansi.Strip(StatCards(styles.Default(), stats, 80))
	if n := len(strings.Split(got, "\n")); n != 5 {
		t.Errorf("row is %d lines, want 5 (border, label, value, hint, border):\n%s", n, got)
	}
	if last := strings.Split(got, "\n")[4]; strings.Count(last, "╯") != 2 {
		t.Errorf("both cards should close on the last line:\n%s", got)
	}
}
