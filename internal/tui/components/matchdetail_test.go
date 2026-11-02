package components_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestShortNameIsTheSurname(t *testing.T) {
	for in, want := range map[string]string{
		"Kai Havertz": "Havertz", "Virgil van Dijk": "van Dijk", "Rodri": "Rodri",
		"Kevin De Bruyne": "De Bruyne", "Alexis Mac Allister": "Mac Allister", "Van Persie": "Persie", "": "",
	} {
		if got := components.ShortName(in); got != want {
			t.Errorf("ShortName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScorersListOneSide(t *testing.T) {
	ev := []models.MatchEvent{
		{Kind: models.EventGoal, Side: models.SideAway, Minute: 16, Player: "Kai Havertz"},
		{Kind: models.EventOwnGoal, Side: models.SideHome, Minute: 30, Player: "Ben White"},
		{Kind: models.EventPenaltyGoal, Side: models.SideAway, Minute: 90, Added: 3, Player: "Bukayo Saka"},
		{Kind: models.EventYellow, Side: models.SideAway, Minute: 40, Player: "Declan Rice"},
		{Kind: models.EventGoal, Side: models.SideAway, Period: 5, Player: "Shoot Out"},
	}
	if got := components.Scorers(ev, models.SideAway); got != "Havertz 16', Saka 90+3' (pen)" {
		t.Errorf("away %q", got)
	}
	if got := components.Scorers(ev, models.SideHome); got != "White 30' (og)" {
		t.Errorf("home %q", got)
	}
}

func TestTimelinePutsEventsOnTheAxis(t *testing.T) {
	th := styles.Plain()
	ev := []models.MatchEvent{
		{Kind: models.EventGoal, Side: models.SideHome, Period: 1, Minute: 1},
		{Kind: models.EventHalfTime, Period: 1, Minute: 45},
		{Kind: models.EventYellow, Side: models.SideAway, Period: 2, Minute: 90},
	}
	got := components.Timeline(th, ev, models.SideHome, 60)
	if lipgloss.Width(got) != 60 {
		t.Errorf("width %d, want 60: %q", lipgloss.Width(got), got)
	}
	if !strings.HasPrefix(got, "0' ─G") || !strings.HasSuffix(got, "y 90'") || !strings.Contains(got, "┼") {
		t.Errorf("axis %q", got)
	}
	extra := append(ev, models.MatchEvent{Kind: models.EventGoal, Side: models.SideAway, Period: 4, Minute: 118})
	if got := components.Timeline(th, extra, models.SideHome, 60); !strings.HasSuffix(got, " 120'") {
		t.Errorf("extra time axis %q", got)
	}
	if components.Timeline(th, ev, "", 12) != "" {
		t.Error("an axis with no room")
	}
}

func TestSplitBarSharesTheWidth(t *testing.T) {
	th := styles.Plain()
	if got := components.SplitBar(th, 25, 75, 8); got != "##......" {
		t.Errorf("bar %q", got)
	}
	if components.SplitBar(th, 0, 0, 8) != "" {
		t.Error("a bar for nothing")
	}
}
