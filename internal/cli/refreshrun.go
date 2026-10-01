package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// tuiRefresher adapts runRefresh to the shape the TUI's refresh queue
// needs: one competition and some of its seasons (the current one when
// none are named), progress as lines of text, and a one-line summary or an
// error at the end.
func tuiRefresher(store storage.Repositories) core.Refresher {
	return func(ctx context.Context, req core.RefreshRequest, progress func(string)) (string, error) {
		comp, ok := sync.FindCompetition(req.CompetitionID)
		if !ok {
			return "", fmt.Errorf("footy doesn't cover %q — see `footy competitions`", req.CompetitionID)
		}
		seasons := req.Seasons
		if len(seasons) == 0 {
			seasons = sync.RecentSeasonYears(time.Now(), 1)
		}

		done := 0
		results := runRefresh(ctx, store, []models.Competition{comp}, seasons, 0, refreshEvents{
			unit: func(u sync.UnitResult) {
				done++
				if len(seasons) == 1 {
					return
				}
				outcome := "ok"
				if u.Failed() {
					outcome = "failed"
				}
				progress(fmt.Sprintf("%s %s %s (%d of %d)",
					comp.ID, components.SeasonName(u.Season), outcome, done, len(seasons)))
			},
			retry: func(attempt, of int, err error) {
				progress(fmt.Sprintf("retry %d of %d: %v", attempt, of, tuiError(err)))
			},
			note: progress,
		})
		summary, err := summarizeUnits(results)
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

// summarizeUnits turns a refresh of one competition into the line the TUI
// shows. Several seasons are summed; the refresh only errors when none of
// them could be fetched, since a partial download is still worth keeping.
func summarizeUnits(results []sync.UnitResult) (string, error) {
	switch len(results) {
	case 0:
		return "", errors.New("nothing was refreshed")
	case 1:
		return summarizeUnit(results[0])
	}

	sum := sync.Summarize(results)
	if sum.Succeeded == 0 && sum.Fresh == 0 {
		for _, r := range results {
			if r.Err != nil {
				return "", r.Err
			}
		}
	}
	line := fmt.Sprintf("%d of %d seasons — %d teams, %d players",
		sum.Succeeded+sum.Fresh, sum.Units, sum.Totals.TeamStats, sum.Totals.PlayerStats)
	var failed []string
	for _, r := range results {
		if r.Failed() {
			failed = append(failed, components.SeasonName(r.Season))
		}
	}
	if len(failed) > 0 {
		line += "; failed: " + strings.Join(failed, ", ")
	}
	return line, nil
}

// summarizeUnit describes one competition-season's refresh.
func summarizeUnit(r sync.UnitResult) (string, error) {
	switch {
	case r.Failed():
		return "", r.Err
	case r.Fresh:
		return "already fresh, cached " + components.Freshness(r.CachedAt, time.Now()), nil
	default:
		return fmt.Sprintf("%s — %d teams, %d players",
			components.SeasonName(r.Season), r.Result.Teams, r.Result.PlayerStats), nil
	}
}
