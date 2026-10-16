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
