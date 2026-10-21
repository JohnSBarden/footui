// Package sources is the one place footui builds its data sources. Which
// source covers a competition is set row by row in sync.Coverages; the
// rest — players' season detail and careers, international matches, the
// player search — come from the people source, sync.DetailProvider.
//
// Adding a source is a package implementing internal/data/provider's
// interfaces, a case in For (and in About), and its coverage rows.
// Nothing outside this package constructs a source or names its package.
package sources

import (
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/provider/espn"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
)

// Retry reports each retried request, for progress output; nil reports
// nothing.
type Retry func(attempt, of int, err error)

// For builds the source of the competitions a coverage row routes to name.
func For(name string, retry Retry) (sync.Source, error) {
	switch name {
	case sync.ProviderUnderstat:
		source := understat.New()
		if retry != nil {
			source = source.OnRetry(retry)
		}
		return source, nil
	case sync.ProviderESPN:
		return people(retry), nil
	}
	return nil, fmt.Errorf("no client for provider %q in this build", name)
}

// people builds the source players and national teams are looked up at,
// whose IDs are recorded under sync.DetailProvider.
func people(retry Retry) *espn.Provider {
	source := espn.New()
	if retry != nil {
		source = source.OnRetry(retry)
	}
	return source
}

// Details builds the source of players' season detail.
func Details() sync.DetailSource { return people(nil) }

// Careers builds the source of the seasons a player has stats in.
func Careers() sync.CareerSource { return people(nil) }

// Matches builds the source of international matches.
func Matches(retry Retry) sync.MatchSource { return people(retry) }

// Players builds the search for players who aren't cached.
func Players() provider.PlayerSearcher { return people(nil) }

// PausedUntil is when footui may ask its sources again after one
// rate-limited it through every retry, or zero when none is paused.
func PausedUntil() time.Time { return espn.PausedUntil() }

// About says what a source's competitions bring beyond the table, for
// the competitions listing.
func About(name string) string {
	switch name {
	case sync.ProviderUnderstat:
		return "expected goals (xG)"
	case sync.ProviderESPN:
		return "possession, corners, and fouls"
	}
	return ""
}
