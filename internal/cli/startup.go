package cli

import (
	"context"
	"strconv"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
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
// moments ago — opening footui twice while understat is down shouldn't
// mean two doomed attempts.
const failureBackoff = 10 * time.Minute

// tuiCatalog lists every competition footui can fetch, with every season
// the provider offers, for the league catalog screen.
//
// International competitions follow the clubs: tournaments by edition,
// qualifiers and nations leagues by calendar year. Friendlies aren't
// offered on their own — they come with a nation — and nor are domestic
// cups, which come with a club. Each gender gets a
// national teams line, whose years are every international competition's
// match lists for that year.
func tuiCatalog(now time.Time) []core.CatalogEntry {
	out := make([]core.CatalogEntry, 0, len(sync.Coverages)+2)
	for _, c := range sync.Coverages {
		if c.Competition.Category == models.CategoryFriendly || c.Competition.Cup() {
			continue
		}
		out = append(out, core.CatalogEntry{Competition: c.Competition, Seasons: c.AvailableSeasons(now),
			ByYear: c.Competition.International() && c.Editions == nil})
	}
	for _, gender := range []string{"men", "women"} {
		entry := core.CatalogEntry{
			Competition: models.Competition{ID: "national-teams-" + gender, Name: "National teams (" + gender + ")",
				Country: "International", Gender: gender, CalendarYear: true},
			Nations: gender,
			Units:   map[string][]string{},
		}
		first := now.Year()
		for _, c := range sync.InternationalCoverages(gender) {
			first = min(first, c.FirstSeason)
		}
		for y := now.Year(); y >= first; y-- {
			units := sync.NationUnits(gender, []int{y})
			if len(units) == 0 {
				continue
			}
			label := strconv.Itoa(y)
			entry.Seasons = append(entry.Seasons, label)
			for _, u := range units {
				entry.Units[label] = append(entry.Units[label], u.Competition.ID)
			}
		}
		out = append(out, entry)
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
// years old was downloaded for its history, and is left alone — unless
// it's a favourite league, which is always followed.
//
// The leagues of favourite clubs and players need no rule of their own: a
// club with a line in this season or last is in a league whose newest
// cached season is one of those two, which the rule above already follows.
func staleRefreshes(ctx context.Context, repos storage.Repositories, now time.Time, maxAge time.Duration) []core.RefreshRequest {
	if maxAge <= 0 {
		return nil
	}
	// A provider that rate-limited footui recently gets left alone: nothing
	// automatic runs until the pause is over.
	if at, err := repos.LastRateLimited(ctx); err == nil && now.Sub(at) < rateLimitPause {
		return nil
	}
	comps, err := repos.ListCompetitions(ctx)
	if err != nil {
		// The TUI will surface a broken database itself; a missed
		// background refresh is not worth refusing to start over.
		return nil
	}
	favorites := favoriteLeagues(ctx, repos)

	var out []core.RefreshRequest
	for _, comp := range comps {
		cov, covered := sync.FindCoverage(comp.ID)
		if !covered || comp.International() || comp.Cup() {
			// International competitions follow the nations and
			// tournaments starred, below; cups follow starred clubs.
			continue
		}
		// Each competition's own calendar: in February MLS's current
		// season is last year's, the Premier League's is this one.
		current := cov.CurrentSeason(now)
		previous := previousSeason(current)
		newest := newestDownloaded(ctx, repos, comp.ID)
		if newest != current && newest != previous && !favorites[comp.ID] {
			continue
		}

		why, stale := currentSeasonStaleness(ctx, repos, comp.ID, sync.NewSeason(comp.ID, current).ID, now, maxAge)
		if !stale {
			continue
		}
		if newest == previous {
			why = "new season " + components.SeasonLabelFor(comp.CalendarYear, current)
		}
		out = append(out, core.RefreshRequest{CompetitionID: comp.ID, Seasons: []string{current}, Why: why, Automatic: true})
	}
	out = append(out, internationalRefreshes(ctx, repos, now, maxAge)...)
	return append(out, clubRefreshes(ctx, repos, now, maxAge)...)
}

// newestDownloaded is the newest season of a competition that was
// downloaded as a season — refreshed for its table — or "" if none was.
// A season row can also exist because a club's matches were fetched from
// it (a Champions League tie), which is not someone following the
// competition.
func newestDownloaded(ctx context.Context, repos storage.Repositories, compID string) string {
	seasons, err := repos.ListSeasons(ctx, compID)
	if err != nil {
		return ""
	}
	for _, se := range seasons {
		if _, err := repos.LatestSyncRun(ctx, compID, se.ID); err == nil {
			return se.Label
		}
	}
	return ""
}

// rateLimitPause is how long automatic refreshes stand down after a
// provider rate-limited footui.
const rateLimitPause = time.Hour

// internationalRefreshes is the stale check for international football:
//
//   - the directory of national teams, once, so search finds every nation
//   - for each gender with a starred nation, this year's and last year's
//     match lists, when any is due by the politeness rules (usually none)
//   - for each starred international competition, its current season
//
// Clubs' rule — follow what was downloaded recently — doesn't apply: a
// nation's matches are downloaded for the nations someone stars.
func internationalRefreshes(ctx context.Context, repos storage.Repositories, now time.Time, maxAge time.Duration) []core.RefreshRequest {
	var out []core.RefreshRequest
	if nations, err := repos.NationalTeams(ctx, ""); err == nil && len(nations) == 0 {
		out = append(out, core.RefreshRequest{Directory: true, Why: "so search can find every nation", Automatic: true})
	}
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		return out
	}
	starred := map[string][]models.Team{}
	for _, f := range favs {
		switch f.Kind {
		case models.FavoriteTeam:
			if t, err := repos.GetTeam(ctx, f.ID); err == nil && t.National {
				starred[t.Gender] = append(starred[t.Gender], t)
			}
		case models.FavoriteCompetition:
			cov, ok := sync.FindCoverage(f.ID)
			if !ok || !cov.Competition.International() {
				continue
			}
			season := cov.CurrentSeason(now)
			year, _ := strconv.Atoi(season)
			if why, due := sync.UnitDue(ctx, repos, sync.Unit{Competition: cov.Competition, Year: year}, now, maxAge); due {
				out = append(out, core.RefreshRequest{CompetitionID: f.ID, Seasons: []string{season}, Why: why, Automatic: true})
			}
		}
	}
	years := sync.RecentYears(now, 2)
	var labels []string
	for _, y := range years {
		labels = append(labels, strconv.Itoa(y))
	}
	for _, gender := range []string{"men", "women"} {
		teams := starred[gender]
		if len(teams) == 0 {
			continue
		}
		for _, u := range sync.NationUnits(gender, years) {
			why, due := sync.UnitDue(ctx, repos, u, now, maxAge)
			if !due {
				continue
			}
			req := core.RefreshRequest{Nations: gender, Seasons: labels, Why: why, Automatic: true}
			if len(teams) == 1 {
				req = core.RefreshRequest{TeamID: teams[0].ID, Name: teams[0].Label(), Seasons: labels, Why: why, Automatic: true}
			}
			out = append(out, req)
			break
		}
	}
	return out
}

// favoriteLeagues is the set of favourite competition IDs. An unreadable
// favourites table means none: the stale check is best-effort.
func favoriteLeagues(ctx context.Context, repos storage.Repositories) map[string]bool {
	out := map[string]bool{}
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		return out
	}
	for _, f := range favs {
		if f.Kind == models.FavoriteCompetition {
			out[f.ID] = true
		}
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

// previousSeason is the start year before season.
func previousSeason(season string) string {
	y, err := strconv.Atoi(season)
	if err != nil {
		return ""
	}
	return strconv.Itoa(y - 1)
}
