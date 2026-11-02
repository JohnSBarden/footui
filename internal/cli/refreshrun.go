package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/colors"
	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sources"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/trophies"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// refreshEvents is how a refresh reports progress. Any field may be nil.
type refreshEvents struct {
	unit  func(sync.UnitResult)
	retry func(attempt, of int, err error)
	// note carries the outcome of the colours and honours re-seeding.
	note func(text string)
}

// runRefresh is the refresh pipeline shared by `footui refresh` and the
// TUI. Re-seeding colours and honours must never fail the refresh. Each
// provider gets its own syncer so one being down doesn't count against
// the other's failure streak.
func runRefresh(ctx context.Context, store storage.Repositories, jobs []sync.Job,
	maxAge time.Duration, ev refreshEvents) []sync.UnitResult {

	var results []sync.UnitResult
	var clubs []sync.Job
	for _, j := range jobs {
		if !j.Competition.International() {
			clubs = append(clubs, j)
			continue
		}
		ms := newMatchSyncer(store, ev)
		ms.StaleAfter = maxAge
		for _, season := range j.Seasons {
			if rateLimited(results) {
				break // ESPN asked footui to stop; the client refuses anyway
			}
			results = append(results, ms.SyncEdition(ctx, j.Competition, season, ev.unitFunc())...)
		}
	}
	results = append(results, syncLeagues(ctx, store, clubs, ev, func(s *sync.Syncer) { s.MaxAge = maxAge })...)
	reseed(ctx, store, ev)
	return results
}

// runHistory fetches league seasons only, keeping finished ones already
// fetched.
func runHistory(ctx context.Context, store storage.Repositories, jobs []sync.Job, ev refreshEvents) []sync.UnitResult {
	results := syncLeagues(ctx, store, jobs, ev, func(s *sync.Syncer) { s.KeepFinished = true })
	reseed(ctx, store, ev)
	return results
}

func syncLeagues(ctx context.Context, store storage.Repositories, jobs []sync.Job, ev refreshEvents,
	configure func(*sync.Syncer)) []sync.UnitResult {

	var results []sync.UnitResult
	for _, group := range groupByProvider(jobs) {
		source, err := newSource(group.provider, ev)
		if err != nil {
			results = append(results, failAll(group.jobs, err, ev)...)
			continue
		}
		syncer := sync.New(source, store).WithProviderName(group.provider)
		configure(syncer)
		results = append(results, syncer.SyncJobs(ctx, group.jobs, ev.unitFunc())...)
	}
	return results
}

func (ev refreshEvents) unitFunc() sync.Progress {
	return func(u sync.UnitResult) {
		if ev.unit != nil {
			ev.unit(u)
		}
	}
}

// newDetailSource, newCareerSource and newMatchSource are variables so
// tests can swap in stubs.
var newDetailSource = sources.Details

var newCareerSource = sources.Careers

var newMatchSource = func(ev refreshEvents) sync.MatchSource {
	return sources.Matches(ev.retry)
}

func newMatchSyncer(store storage.Repositories, ev refreshEvents) *sync.MatchSyncer {
	return sync.NewMatchSyncer(newMatchSource(ev), store).WithProviderName(sync.DetailProvider)
}

// runNations: zero staleAfter fetches whatever has changed.
func runNations(ctx context.Context, store storage.Repositories, gender string, years []int,
	staleAfter time.Duration, ev refreshEvents) []sync.UnitResult {

	ms := newMatchSyncer(store, ev)
	ms.StaleAfter = staleAfter
	return ms.SyncNations(ctx, gender, years, ev.unitFunc())
}

// seedDirectory stores both genders' national teams so search finds them
// before any matches are fetched.
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

// groupByProvider preserves the requested order within and across groups.
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

func newSource(provider string, ev refreshEvents) (sync.Source, error) {
	return sources.For(provider, ev.retry)
}

// failAll fails every unit of a provider that couldn't be set up.
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

// reseed applies curated colours and honours to whatever is now cached.
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

// tuiRefresher adapts runRefresh to the TUI's refresh queue. staleAfter
// applies only to automatic refreshes; a user-requested one fetches
// whatever has changed.
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
		case req.PlayerID != "" && req.History:
			return refreshPlayerHistory(ctx, store, req.PlayerID, ev, progress)
		case req.PlayerID != "":
			return refreshPlayer(ctx, store, req.PlayerID, stale)
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
					if req.History {
						return refreshClubHistory(ctx, store, team, req.CompetitionID, ev, progress)
					}
					ev.unit = func(u sync.UnitResult) {
						progress(fmt.Sprintf("%s %s %s", u.Competition.Abbrev(), components.SeasonName(u.Season), unitOutcome(u)))
					}
					results, lastGame, err := runClub(ctx, store, team, stale, ev)
					if err != nil {
						return "", err
					}
					summary, err := summarizeMatches(results, "recent games up to date", false)
					if err != nil {
						return "", err
					}
					return withLastGame(summary, lastGame), nil
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

// refreshPlayer fetches a player's season detail.
func refreshPlayer(ctx context.Context, store storage.Repositories, playerID string, maxAge time.Duration) (string, error) {
	res, err := sync.RefreshPlayerDetail(ctx, store, newDetailSource(), playerID, time.Now(), maxAge)
	if errors.Is(err, provider.ErrRateLimited) {
		return "", core.ErrRateLimited
	}
	if err != nil {
		return "", tuiError(err)
	}
	switch res.Fetched {
	case 0:
		return "stats up to date", nil
	case 1:
		return "1 season of stats refreshed", nil
	}
	return fmt.Sprintf("%d seasons of stats refreshed", res.Fetched), nil
}

// refreshPlayerHistory looks seasons up by ESPN ID, so a player first seen
// on understat gets a detail refresh first to resolve it.
func refreshPlayerHistory(ctx context.Context, store storage.Repositories, playerID string,
	ev refreshEvents, progress func(string)) (string, error) {

	if _, err := store.ProviderID(ctx, "player", playerID, sync.DetailProvider); errors.Is(err, storage.ErrNotFound) {
		if _, err := refreshPlayer(ctx, store, playerID, 0); err != nil {
			return "", err
		}
	}
	jobs, err := sync.PlayerCareerJobs(ctx, store, newCareerSource(), playerID, time.Now())
	if errors.Is(err, provider.ErrRateLimited) {
		return "", core.ErrRateLimited
	}
	if errors.Is(err, sync.ErrNoCareer) {
		detail, err := refreshPlayer(ctx, store, playerID, 0)
		if err != nil {
			return "", err
		}
		return detail + "; their other seasons couldn't be looked up", nil
	}
	if err != nil {
		return "", tuiError(err)
	}
	if scope := historyScope(ctx, store, jobs); scope != "" {
		progress(scope)
	}
	ev.unit = historyProgress(sync.Units(jobs), progress)
	results := runHistory(ctx, store, jobs, ev)
	if rateLimited(results) {
		return "", core.ErrRateLimited
	}
	detail, err := refreshPlayer(ctx, store, playerID, 0)
	if err != nil {
		return "", err
	}
	return historySummary(results) + "; " + detail, nil
}

// refreshClubHistory fetches every offered season of each league a club
// has a table in, then its recent games.
func refreshClubHistory(ctx context.Context, store storage.Repositories, club models.Team, fromCompetitionID string,
	ev refreshEvents, progress func(string)) (string, error) {

	var also []string
	if fromCompetitionID != "" {
		also = []string{fromCompetitionID}
	}
	jobs, err := sync.ClubHistoryJobs(ctx, store, club.ID, also, time.Now())
	if err != nil {
		return "", err
	}
	if scope := historyScope(ctx, store, jobs); scope != "" {
		progress(scope)
	}
	ev.unit = historyProgress(sync.Units(jobs), progress)
	results := runHistory(ctx, store, jobs, ev)
	if rateLimited(results) {
		return "", core.ErrRateLimited
	}
	ev.unit = func(u sync.UnitResult) {
		progress(fmt.Sprintf("%s %s %s", u.Competition.Abbrev(), components.SeasonName(u.Season), unitOutcome(u)))
	}
	games, lastGame, err := runClub(ctx, store, club, 0, ev)
	if err != nil {
		return "", err
	}
	recent, err := summarizeMatches(games, "recent games up to date", false)
	if err != nil {
		return "", err
	}
	return historySummary(results) + "; " + withLastGame(recent, lastGame), nil
}

// withLastGame adds what the last-game step fetched to a club summary;
// "nothing to fetch" becomes just that.
func withLastGame(summary, lastGame string) string {
	switch {
	case lastGame == "":
		return summary
	case summary == "nothing to fetch" || summary == "recent games up to date":
		return lastGame
	}
	return summary + "; " + lastGame
}

// seasonTime is roughly how long a season takes to fetch, measured: ESPN
// makes about 60 requests a season, understat one.
var seasonTime = map[string]time.Duration{
	sync.ProviderESPN:      25 * time.Second,
	sync.ProviderUnderstat: 3 * time.Second,
}

// historyScope says up front how big a history run is, since the first
// season can take a while to land: "30 seasons to fetch, 5 cached —
// about 12 minutes". Empty when everything is cached.
func historyScope(ctx context.Context, store storage.Repositories, jobs []sync.Job) string {
	var estimate time.Duration
	fetch := 0
	for _, g := range groupByProvider(jobs) {
		n := sync.PendingUnits(ctx, store, g.jobs)
		fetch += n
		estimate += time.Duration(n) * seasonTime[g.provider]
	}
	if fetch == 0 {
		return ""
	}
	scope := components.Plural(fetch, "season") + " to fetch"
	if cached := sync.Units(jobs) - fetch; cached > 0 {
		scope += fmt.Sprintf(", %d cached", cached)
	}
	return scope + " — " + roughly(estimate)
}

func roughly(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < 90*time.Second:
		return "about a minute"
	}
	return fmt.Sprintf("about %d minutes", int(d.Minutes()+0.5))
}

func historyProgress(total int, progress func(string)) func(sync.UnitResult) {
	done := 0
	return func(u sync.UnitResult) {
		done++
		progress(fmt.Sprintf("%s %s %s (%d of %d)",
			u.Competition.Abbrev(), components.SeasonName(u.Season), unitOutcome(u), done, total))
	}
}

// historySummary says how much of a history was fetched: "12 seasons
// fetched, 13 already cached".
func historySummary(results []sync.UnitResult) string {
	sum := sync.Summarize(results)
	if sum.Units == 0 {
		return "no league seasons to fetch"
	}
	seasons := func(n int) string {
		if n == 1 {
			return "1 season"
		}
		return fmt.Sprintf("%d seasons", n)
	}
	var parts []string
	if sum.Succeeded > 0 || sum.Fresh == 0 {
		parts = append(parts, seasons(sum.Succeeded)+" fetched")
	}
	if sum.Fresh > 0 {
		parts = append(parts, fmt.Sprintf("%d already cached", sum.Fresh))
	}
	if sum.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", sum.Failed))
	}
	return strings.Join(parts, ", ")
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

// summarizeMatches returns core.ErrRateLimited so the app stops its
// queue; otherwise it errors only when nothing could be fetched.
// nameFailures lists failed units rather than counting them.
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

// tuiError shortens a refresh failure to its cause and attempt count for
// the TUI's one-line status bar. Unrecognised errors pass through.
func tuiError(err error) error {
	var msg string
	var attempts int
	var timeout provider.Timeout
	switch {
	case errors.As(err, &timeout):
		msg = fmt.Sprintf("no response within %s", timeout.TimedOutAfter())
		var ex provider.Exhausted
		if errors.As(err, &ex) {
			attempts = ex.AttemptsMade()
		}
	case errors.Is(err, provider.ErrRateLimited):
		return core.ErrRateLimited
	case errors.Is(err, provider.ErrNoTable):
		return errors.New("no table for that season yet")
	default:
		return err
	}
	if attempts > 0 {
		msg += fmt.Sprintf(" (%d attempts)", attempts)
	}
	return errors.New(msg)
}

// summarizeUnits errors only when no season could be fetched; a partial
// download is still worth keeping.
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
