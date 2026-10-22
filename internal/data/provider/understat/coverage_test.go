package understat

import (
	"testing"

	"github.com/JohnSBarden/footui/internal/data/sync"
)

// Every competition the registry routes here must be fetchable here.
func TestEveryRoutedCompetitionHasALeagueKey(t *testing.T) {
	for _, c := range sync.Coverages {
		if c.Provider != sync.ProviderUnderstat {
			continue
		}
		if _, ok := leagueKeys[c.Competition.ID]; !ok {
			t.Errorf("%s is routed to understat but has no leagueKeys entry", c.Competition.ID)
		}
	}
}
