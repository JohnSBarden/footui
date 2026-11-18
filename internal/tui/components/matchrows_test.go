package components

import (
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// A fixture's score cell is its kickoff time; the date has its own column.
func TestFixtureShowsKickoffTimeOnly(t *testing.T) {
	kickoff := time.Date(2026, 12, 5, 17, 30, 0, 0, time.Local)
	m := models.Match{HomeTeamID: "nat-england", AwayTeamID: "nat-ukraine", Kickoff: kickoff}
	cells := MatchCells(styles.Default(), m, "nat-england", models.Competition{}, "Ukraine", "")
	if got := cells[1].Text; got != "2026-12-05" {
		t.Errorf("date = %q", got)
	}
	if got := cells[5].Text; got != "17:30" {
		t.Errorf("score = %q, want the kickoff time", got)
	}
}
