package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/colors"
	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider/understat"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/trophies"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// refreshEvents is how a refresh reports as it runs. The CLI prints each
// to stdout; the TUI folds them into its status line. Any may be nil.
type refreshEvents struct {
	// unit is called as each competition-season finishes.
	unit func(sync.UnitResult)
	// retry is called each time a failed request is about to be retried.
	retry func(attempt, of int, err error)
	// note carries the outcome of the colors and honours re-seeding.
	note func(text string)
}

// runRefresh is the one refresh pipeline, shared by `footy refresh` and
// the TUI's refresh key: fetch each competition-season from understat,
// then re-seed curated colors and honours so a newly cached competition
// picks up its clubs' branding and trophies with no extra step. Neither
// seeding step may fail the refresh: the stats are the point, these are
// decoration.
func runRefresh(ctx context.Context, store storage.Repositories, comps []models.Competition,
	seasons []string, maxAge time.Duration, ev refreshEvents) []sync.UnitResult {

	source := understat.New()
	if ev.retry != nil {
		source = source.OnRetry(ev.retry)
	}
	syncer := sync.New(source, store).WithProviderName("understat")
	syncer.MaxAge = maxAge

	results := syncer.SyncMany(ctx, comps, seasons, func(u sync.UnitResult) {
		if ev.unit != nil {
			ev.unit(u)
		}
	})

	note := func(text string) {
		if ev.note != nil {
			ev.note(text)
		}
	}
	if colorReport, err := colors.Seed(ctx, store); err != nil {
		note(fmt.Sprintf("note: could not apply club colors: %v", err))
	} else if colorReport.Applied > 0 {
		note(fmt.Sprintf("colors applied to %d of %d cached teams",
			colorReport.Applied, colorReport.Applied+len(colorReport.Uncolored)))
	}
	if trophyReport, err := trophies.Seed(ctx, store); err != nil {
		note(fmt.Sprintf("note: could not apply club honours: %v", err))
	} else if trophyReport.Teams > 0 {
		note(fmt.Sprintf("honours applied to %d teams (%d trophies)",
			trophyReport.Teams, trophyReport.Trophies))
	}

	return results
}

// tuiRefresher adapts runRefresh to the shape the TUI's refresh key needs:
// one competition, the current season, progress as lines of text, and a
// one-line summary or an error at the end.
func tuiRefresher(store storage.Repositories) core.Refresher {
	return func(ctx context.Context, competitionID string, progress func(string)) (string, error) {
		comp, ok := sync.FindCompetition(competitionID)
		if !ok {
			return "", fmt.Errorf("footy doesn't cover %q — see `footy competitions`", competitionID)
		}
		seasons := sync.RecentSeasonYears(time.Now(), 1)

		results := runRefresh(ctx, store, []models.Competition{comp}, seasons, 0, refreshEvents{
			retry: func(attempt, of int, err error) {
				progress(fmt.Sprintf("retry %d of %d: %v", attempt, of, tuiError(err)))
			},
			note: progress,
		})
		summary, err := summarizeUnit(results)
		return summary, tuiError(err)
	}
}

// tuiError rephrases a refresh failure for the TUI's one-line status bar.
// The CLI prints the whole wrapped chain ("sync: fetch teams: giving up
// after 3 attempts: understat: fetch EPL 2026: no response …"), but the
// TUI shows the error beneath a screen that already says what was being
// refreshed, so only the cause and the attempt count are kept. Errors it
// doesn't recognise pass through unchanged.
func tuiError(err error) error {
	var timeout *understat.TimeoutError
	if !errors.As(err, &timeout) {
		return err
	}
	msg := fmt.Sprintf("no response from %s within %s", timeout.Host, timeout.After)
	var exhausted *understat.ExhaustedError
	if errors.As(err, &exhausted) {
		msg += fmt.Sprintf(" (%d attempts)", exhausted.Attempts)
	}
	return errors.New(msg)
}

// summarizeUnit turns a single-unit refresh into the line the TUI shows.
func summarizeUnit(results []sync.UnitResult) (string, error) {
	if len(results) != 1 {
		return "", fmt.Errorf("expected one competition-season, got %d", len(results))
	}
	r := results[0]
	switch {
	case r.Failed():
		return "", r.Err
	case r.Fresh:
		return "already fresh, cached " + components.Freshness(r.CachedAt, time.Now()), nil
	default:
		return fmt.Sprintf("%s — %d teams, %d players",
			seasonDisplay(r.Season.Label), r.Result.Teams, r.Result.PlayerStats), nil
	}
}
