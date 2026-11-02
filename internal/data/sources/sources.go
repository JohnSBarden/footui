// Package sources is the one place footui builds its data sources. Adding
// one means a case in For and About plus its rows in sync.Coverages.
package sources

import (
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/provider/espn"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
)

// Retry reports each retried request; nil reports nothing.
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

// people builds the source whose IDs are recorded under
// sync.DetailProvider.
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

// MatchDetails builds the source of a last game's events, stats, lineups
// and commentary.
func MatchDetails(retry Retry) sync.LastGameSource { return people(retry) }

// Players builds the search for players who aren't cached.
func Players() provider.PlayerSearcher { return people(nil) }

// PausedUntil is when a rate-limited source may be asked again, or zero.
func PausedUntil() time.Time { return espn.PausedUntil() }

// About says what a source's competitions bring beyond the table.
func About(name string) string {
	switch name {
	case sync.ProviderUnderstat:
		return "expected goals (xG)"
	case sync.ProviderESPN:
		return "possession, corners, and fouls"
	}
	return ""
}
