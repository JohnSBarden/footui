package sources

import (
	"testing"

	"github.com/JohnSBarden/footui/internal/data/sync"
)

// Every provider a coverage row routes to has a source and a line in the
// competitions listing.
func TestEveryCoveredProviderHasASource(t *testing.T) {
	for _, c := range sync.Coverages {
		if _, err := For(c.Provider, nil); err != nil {
			t.Errorf("%s: %v", c.Competition.ID, err)
		}
		if About(c.Provider) == "" {
			t.Errorf("%s: no About for %q", c.Competition.ID, c.Provider)
		}
	}
	if _, err := For("nowhere", nil); err == nil {
		t.Error("For built a source for an unknown provider")
	}
}
