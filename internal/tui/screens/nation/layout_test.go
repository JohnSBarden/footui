package nation_test

import (
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

// A narrow terminal shows the years around the selected one, never a
// run of old years with the selection cut off the end.
func TestNarrowYearBarKeepsTheSelectedYear(t *testing.T) {
	m, _ := open(t)
	out := tuitest.Resize(t, m, 24, 30).View()
	if !strings.Contains(out, "[2026]") {
		t.Errorf("the selected year scrolled out of the bar\n---\n%s", out)
	}
}

func TestNationFitsTheTerminal(t *testing.T) {
	m, _ := open(t)
	for _, size := range [][2]int{{80, 19}, {120, 35}, {200, 50}} {
		out := tuitest.Resize(t, m, size[0], size[1]).View()
		if got := strings.Count(out, "\n") + 1; got > size[1] {
			t.Errorf("%dx%d: drew %d lines\n---\n%s", size[0], size[1], got, out)
		}
	}
}
