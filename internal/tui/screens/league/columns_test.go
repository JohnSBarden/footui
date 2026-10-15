package league

import (
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func titles(m *Model) map[string]bool {
	out := map[string]bool{}
	for _, c := range m.columns() {
		out[c.Title] = true
	}
	return out
}

// A competition without expected goals (anything from ESPN) shows what it
// does have instead of three columns of dashes.
func TestColumnsFollowTheDataTheCompetitionHas(t *testing.T) {
	m := New(core.Deps{Theme: styles.Plain()}, "usa-mls")
	m.width = 140

	m.rows = []Row{{Stats: models.TeamSeasonStats{PossessionAvg: new(55.2), Shots: new(400), ShotsOnTarget: new(150)}}}
	got := titles(m)
	if got["xG"] || got["xGD"] {
		t.Errorf("xG columns shown for a table with no xG: %v", got)
	}
	if !got["Poss"] || !got["Sh"] || !got["SoT"] {
		t.Errorf("possession and shots should be shown: %v", got)
	}

	m.rows = []Row{{Stats: models.TeamSeasonStats{XG: new(50.0), XGA: new(30.0)}}}
	got = titles(m)
	if !got["xG"] || !got["xGA"] || !got["xGD"] || got["Poss"] {
		t.Errorf("an understat table keeps its xG columns: %v", got)
	}

	m.rows = []Row{{Stats: models.TeamSeasonStats{}}}
	if got := titles(m); len(got) != 10 {
		t.Errorf("with nothing optional, only the base table: %v", got)
	}
}
