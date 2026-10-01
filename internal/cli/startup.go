package cli

import (
	"context"
	"time"

	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// DefaultStaleAfter is how old a cached current season may get before
// opening the TUI refreshes it. Six hours keeps a matchday's results from
// going unseen for long without re-fetching on every launch.
const DefaultStaleAfter = 6 * time.Hour

// failureBackoff stops a launch from retrying a refresh that failed only
// moments ago — opening footy twice while understat is down shouldn't
// mean two doomed attempts.
const failureBackoff = 10 * time.Minute

// tuiCatalog lists every competition footy can fetch, with every season
// the provider offers, for the league catalog screen.
func tuiCatalog(now time.Time) []core.CatalogEntry {
	years := sync.AvailableSeasonYears(now)
	out := make([]core.CatalogEntry, 0, len(sync.Scope))
	for _, c := range sync.Scope {
		out = append(out, core.CatalogEntry{Competition: c, Seasons: years})
	}
	return out
}

// staleRefreshes finds the competitions the user follows whose current
// season is out of date, as refresh requests to queue when the TUI opens.
//
// A competition counts as followed when its newest cached season is the
// current one or the one before it. The second case is the season
// rollover: someone who had 2025-26 in May wants 2026-27 in September
// without being told to fetch it. A competition whose newest season is
// years old was downloaded for its history, and is left alone.
func staleRefreshes(ctx context.Context, repos storage.Repositories, now time.Time, maxAge time.Duration) []core.RefreshRequest {
	if maxAge <= 0 {
		return nil
	}
	current := sync.CurrentSeasonStartYear(now)
	previous := sync.RecentSeasonYears(now, 2)[1]

	comps, err := repos.ListCompetitions(ctx)
	if err != nil {
		// The TUI will surface a broken database itself; a missed
		// background refresh is not worth refusing to start over.
		return nil
	}

	var out []core.RefreshRequest
	for _, comp := range comps {
		if _, covered := sync.FindCompetition(comp.ID); !covered {
			continue
		}
		seasons, err := repos.ListSeasons(ctx, comp.ID)
		if err != nil || len(seasons) == 0 {
			continue
		}
		newest := seasons[0].Label
		if newest != current && newest != previous {
			continue
		}

		why, stale := currentSeasonStaleness(ctx, repos, comp.ID, sync.NewSeason(comp.ID, current).ID, now, maxAge)
		if !stale {
			continue
		}
		if newest == previous {
			why = "new season " + components.SeasonLabel(current)
		}
		out = append(out, core.RefreshRequest{CompetitionID: comp.ID, Seasons: []string{current}, Why: why})
	}
	return out
}

// currentSeasonStaleness reports whether one competition-season needs
// refreshing, and a short reason to show the user.
func currentSeasonStaleness(ctx context.Context, repos storage.Repositories, compID, seasonID string,
	now time.Time, maxAge time.Duration) (string, bool) {

	run, err := repos.LatestSyncRun(ctx, compID, seasonID)
	if err != nil {
		// Never attempted (or unreadable): fetch it.
		return "not cached yet", true
	}
	age := now.Sub(run.FinishedAt)
	if !run.Succeeded() {
		if age < failureBackoff {
			return "", false
		}
		return "last attempt failed", true
	}
	if age <= maxAge {
		return "", false
	}
	return "cached " + components.Freshness(run.FinishedAt, now), true
}
