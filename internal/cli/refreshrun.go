package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/colors"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/provider/espn"
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

// runRefresh is the one refresh pipeline, shared by `footui refresh` and
// the TUI's refresh key: fetch each competition-season from the provider
// that covers it, then re-seed curated colors and honours so a newly
// cached competition picks up its clubs' branding and trophies with no
// extra step. Neither seeding step may fail the refresh: the stats are
// the point, these are decoration.
//
// Jobs are grouped by provider, each group with its own syncer, so one
// provider being down doesn't count against the other's failure streak.
func runRefresh(ctx context.Context, store storage.Repositories, jobs []sync.Job,
	maxAge time.Duration, ev refreshEvents) []sync.UnitResult {

	var results []sync.UnitResult
	var clubs []sync.Job
	for _, j := range jobs {
		if !j.Competition.International() {
			clubs = append(clubs, j)
			continue
		}
		// An international season is matches from the shared yearly
		// lists, plus its group tables.
		ms := newMatchSyncer(store, ev)
		ms.StaleAfter = maxAge
		for _, season := range j.Seasons {
			if rateLimited(results) {
				break // ESPN asked footui to stop; the client refuses anyway
			}
			results = append(results, ms.SyncEdition(ctx, j.Competition, season, ev.unitFunc())...)
		}
	}
	for _, group := range groupByProvider(clubs) {
		source, err := newSource(group.provider, ev)
		if err != nil {
			results = append(results, failAll(group.jobs, err, ev)...)
			continue
		}
		syncer := sync.New(source, store).WithProviderName(group.provider)
		syncer.MaxAge = maxAge
		results = append(results, syncer.SyncJobs(ctx, group.jobs, ev.unitFunc())...)
	}
	reseed(ctx, store, ev)
	return results
}

func (ev refreshEvents) unitFunc() sync.Progress {
	return func(u sync.UnitResult) {
		if ev.unit != nil {
			ev.unit(u)
		}
	}
}

// newMatchSource builds the source of international matches. It is a
// variable so tests can point it at a fake server.
var newMatchSource = func(ev refreshEvents) sync.MatchSource {
	source := espn.New()
	if ev.retry != nil {
		source = source.OnRetry(ev.retry)
	}
	return source
}

func newMatchSyncer(store storage.Repositories, ev refreshEvents) *sync.MatchSyncer {
	return sync.NewMatchSyncer(newMatchSource(ev), store).WithProviderName(sync.ProviderESPN)
}

// runNations fetches the yearly match lists a gender's nations play in.
// staleAfter holds the current year to the stale threshold; zero fetches
// whatever has changed.
func runNations(ctx context.Context, store storage.Repositories, gender string, years []int,
	staleAfter time.Duration, ev refreshEvents) []sync.UnitResult {

	ms := newMatchSyncer(store, ev)
	ms.StaleAfter = staleAfter
	return ms.SyncNations(ctx, gender, years, ev.unitFunc())
}

// seedDirectory stores both genders' national teams, so search finds
// every nation before any of its matches are fetched.
func seedDirectory(ctx context.Context, store storage.Repositories, ev refreshEvents) (int, error) {
	ms := newMatchSyncer(store, ev)
	total := 0
	for _, g := range []string{"men", "women"} {
		n, err := ms.SeedNationalTeams(ctx, g)
		if err != nil {
			return total, fmt.Errorf("national teams (%s): %w", g, err)
		}
		total += n
	}
	return total, nil
}

// rateLimited reports whether any unit was refused for asking too often.
func rateLimited(results []sync.UnitResult) bool {
	for _, r := range results {
		if errors.Is(r.Err, provider.ErrRateLimited) {
			return true
		}
	}
	return false
}

type providerJobs struct {
	provider string
	jobs     []sync.Job
}

// groupByProvider splits jobs by the provider covering each, keeping the
// order competitions were asked for within and across groups.
func groupByProvider(jobs []sync.Job) []providerJobs {
	var groups []providerJobs
	index := map[string]int{}
	for _, j := range jobs {
		provider := sync.ProviderUnderstat
		if cov, ok := sync.FindCoverage(j.Competition.ID); ok {
			provider = cov.Provider
		}
		i, ok := index[provider]
		if !ok {
			i = len(groups)
			index[provider] = i
			groups = append(groups, providerJobs{provider: provider})
		}
		groups[i].jobs = append(groups[i].jobs, j)
	}
	return groups
}

// newSource constructs the client for a provider.
func newSource(provider string, ev refreshEvents) (sync.Source, error) {
	switch provider {
	case sync.ProviderUnderstat:
		source := understat.New()
		if ev.retry != nil {
			source = source.OnRetry(ev.retry)
		}
		return source, nil
	case sync.ProviderESPN:
		source := espn.New()
		if ev.retry != nil {
			source = source.OnRetry(ev.retry)
		}
		return source, nil
	}
	return nil, fmt.Errorf("no client for provider %q in this build", provider)
}

// failAll records every unit of jobs as failed with err, for a provider
// that couldn't even be set up.
func failAll(jobs []sync.Job, err error, ev refreshEvents) []sync.UnitResult {
	var out []sync.UnitResult
	for _, j := range jobs {
		for _, y := range j.Seasons {
			u := sync.UnitResult{Competition: j.Competition, Season: sync.NewSeason(j.Competition.ID, y), Err: err}
			out = append(out, u)
			if ev.unit != nil {
				ev.unit(u)
			}
		}
	}
	return out
}

// reseed applies curated colors and honours to whatever is now cached.
func reseed(ctx context.Context, store storage.Repositories, ev refreshEvents) {
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
}

// tuiRefresher adapts runRefresh to the shape the TUI's refresh queue
// needs: one competition and some of its seasons (the current one when
// none are named), progress as lines of text, and a one-line summary or an
// error at the end.
//
// staleAfter is the threshold an automatic refresh of international
// matches is held to; one the user asked for fetches whatever has changed.
func tuiRefresher(store storage.Repositories, staleAfter time.Duration) core.Refresher {
	return func(ctx context.Context, req core.RefreshRequest, progress func(string)) (string, error) {
		ev := refreshEvents{
			retry: func(attempt, of int, err error) {
				progress(fmt.Sprintf("retry %d of %d: %v", attempt, of, tuiError(err)))
			},
			note: progress,
		}
		stale := time.Duration(0)
		if req.Automatic {
			stale = staleAfter
		}
		switch {
		case req.Directory:
			n, err := seedDirectory(ctx, store, ev)
			if err != nil {
				return "", tuiError(err)
			}
			return fmt.Sprintf("%d national teams, ready to search", n), nil
		case req.TeamID != "" || req.Nations != "":
			gender := req.Nations
			if req.TeamID != "" {
				team, err := store.GetTeam(ctx, req.TeamID)
				if err != nil {
					return "", err
				}
				if !team.National {
					// A club: its recent games, from its league and the
					// shared lists of its cups and Europe's competitions.
					ev.unit = func(u sync.UnitResult) {
						progress(fmt.Sprintf("%s %s %s", u.Competition.Abbrev(), components.SeasonName(u.Season), unitOutcome(u)))
					}
					results, err := runClub(ctx, store, team, stale, ev)
					if err != nil {
						return "", err
					}
					return summarizeMatches(results, "recent games up to date", false)
				}
				gender = team.Gender
			}
			years, err := calendarYears(req.Seasons, time.Now())
			if err != nil {
				return "", err
			}
			units := sync.NationUnits(gender, years)
			done := 0
			ev.unit = func(u sync.UnitResult) {
				done++
				outcome := "fetched"
				switch {
				case u.Fresh:
					outcome = "up to date"
				case u.Failed():
					outcome = "failed"
				}
				progress(fmt.Sprintf("%s %s %s (%d of %d)", u.Competition.Abbrev(), u.Season.Label, outcome, done, len(units)))
			}
			return summarizeMatches(runNations(ctx, store, gender, years, stale, ev), "already up to date", true)
		}

		cov, ok := sync.FindCoverage(req.CompetitionID)
		if !ok {
			return "", fmt.Errorf("footui doesn't cover %q — see `footui competitions`", req.CompetitionID)
		}
		comp := cov.Competition
		seasons := req.Seasons
		if len(seasons) == 0 {
			seasons = []string{cov.CurrentSeason(time.Now())}
		}

		if comp.International() {
			ev.unit = func(u sync.UnitResult) {
				progress(fmt.Sprintf("%s %s %s", comp.Abbrev(), components.SeasonName(u.Season), unitOutcome(u)))
			}
			return summarizeMatches(runRefresh(ctx, store, []sync.Job{{Competition: comp, Seasons: seasons}}, stale, ev), "already up to date", true)
		}

		done := 0
		results := runRefresh(ctx, store, []sync.Job{{Competition: comp, Seasons: seasons}}, 0, refreshEvents{
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
		if rateLimited(results) {
			return "", core.ErrRateLimited
		}
		summary, err := summarizeUnits(results)
		return summary, tuiError(err)
	}
}

// calendarYears parses requested years, defaulting to this year and last.
func calendarYears(seasons []string, now time.Time) ([]int, error) {
	if len(seasons) == 0 {
		return sync.RecentYears(now, 2), nil
	}
	out := make([]int, 0, len(seasons))
	for _, s := range seasons {
		y, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("%q isn't a year", s)
		}
		out = append(out, y)
	}
	return out, nil
}

func unitOutcome(u sync.UnitResult) string {
	switch {
	case u.Fresh:
		return "up to date"
	case u.Skipped:
		return "skipped"
	case u.Failed():
		return "failed"
	}
	return "fetched"
}

// summarizeMatches turns a refresh of matches into the line the TUI
// shows: how much was fetched, how much was already up to date — usually
// all of it, which is the politeness rules working — and what failed.
// Being rate-limited is reported as such, so the app stops its queue;
// otherwise it only errors when nothing could be fetched. upToDate is the
// line when nothing needed fetching; nameFailures lists each failed unit
// rather than counting them.
func summarizeMatches(results []sync.UnitResult, upToDate string, nameFailures bool) (string, error) {
	if rateLimited(results) {
		return "", core.ErrRateLimited
	}
	sum := sync.Summarize(results)
	if sum.Units == 0 {
		return "nothing to fetch", nil
	}
	if sum.Succeeded == 0 && sum.Fresh == 0 {
		for _, r := range results {
			if r.Err != nil {
				return "", tuiError(r.Err)
			}
		}
	}
	line := upToDate
	if sum.Succeeded > 0 || sum.Failed > 0 {
		line = fmt.Sprintf("fetched %d, %d matches", sum.Succeeded, sum.Totals.Matches)
		if sum.Fresh > 0 {
			line += fmt.Sprintf("; %d already up to date", sum.Fresh)
		}
	}
	if !nameFailures {
		if sum.Failed > 0 {
			line += fmt.Sprintf("; %d failed", sum.Failed)
		}
		return line, nil
	}
	var failed []string
	for _, r := range results {
		if r.Failed() && !r.Skipped {
			failed = append(failed, r.Competition.Abbrev()+" "+components.SeasonName(r.Season))
		}
	}
	if len(failed) > 0 {
		line += "; failed: " + strings.Join(failed, ", ")
	}
	return line, nil
}

// tuiError rephrases a refresh failure for the TUI's one-line status bar.
// The CLI prints the whole wrapped chain ("sync: fetch teams: giving up
// after 3 attempts: understat: fetch EPL 2026: no response …"), but the
// TUI shows the error beneath a screen that already says what was being
// refreshed, so only the cause and the attempt count are kept. Errors it
// doesn't recognise pass through unchanged.
func tuiError(err error) error {
	var msg string
	var attempts int
	var ut *understat.TimeoutError
	var et *espn.TimeoutError
	switch {
	case errors.As(err, &ut):
		msg = fmt.Sprintf("no response from %s within %s", ut.Host, ut.After)
		var ex *understat.ExhaustedError
		if errors.As(err, &ex) {
			attempts = ex.Attempts
		}
	case errors.As(err, &et):
		msg = fmt.Sprintf("no response from ESPN within %s", et.After)
		var ex *espn.ExhaustedError
		if errors.As(err, &ex) {
			attempts = ex.Attempts
		}
	case errors.Is(err, provider.ErrRateLimited):
		return core.ErrRateLimited
	case errors.Is(err, espn.ErrNoTable):
		return errors.New("ESPN has no table for that season")
	default:
		return err
	}
	if attempts > 0 {
		msg += fmt.Sprintf(" (%d attempts)", attempts)
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
