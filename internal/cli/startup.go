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
// opening the TUI refreshes it.
const DefaultStaleAfter = 6 * time.Hour

// failureBackoff stops a launch retrying a refresh that just failed.
const failureBackoff = 10 * time.Minute

// tuiCatalog lists every fetchable competition and season for the catalog
// screen. Friendlies come with a nation and cups with a club, so neither
// is listed on its own.
func tuiCatalog(now time.Time) []core.CatalogEntry {
	out := make([]core.CatalogEntry, 0, len(sync.Coverages)+2)
	for _, c := range sync.Coverages {
		if c.Competition.Category == models.CategoryFriendly || c.Competition.Cup() {
			continue
		}
		out = append(out, core.CatalogEntry{Competition: c.Competition, Seasons: c.AvailableSeasons(now),
			ByYear: c.Competition.International() && c.Editions == nil, Base: sync.InBase(c.Competition.ID)})
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

// staleRefreshes returns refresh requests to queue when the TUI opens. A
// competition is followed when its newest downloaded season is the current
// or previous one (the latter covers season rollover), or it's a favourite.
func staleRefreshes(ctx context.Context, repos storage.Repositories, now time.Time, maxAge time.Duration) []core.RefreshRequest {
	if maxAge <= 0 {
		return nil
	}
	if at, err := repos.LastRateLimited(ctx); err == nil && now.Sub(at) < rateLimitPause {
		return nil
	}
	comps, err := repos.ListCompetitions(ctx)
	if err != nil {
		// The TUI surfaces a broken database itself.
		return nil
	}
	favorites := favoriteLeagues(ctx, repos)

	var out []core.RefreshRequest
	for _, comp := range comps {
		cov, covered := sync.FindCoverage(comp.ID)
		if !covered || comp.International() || comp.Cup() {
			// Handled by internationalRefreshes and clubRefreshes.
			continue
		}
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
	out = append(out, clubRefreshes(ctx, repos, now, maxAge)...)
	return append(out, playerRefreshes(ctx, repos)...)
}

// playerRefreshes queues starred players after the leagues so tables come
// first; the sync layer skips anything still fresh.
func playerRefreshes(ctx context.Context, repos storage.Repositories) []core.RefreshRequest {
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		return nil
	}
	var out []core.RefreshRequest
	for _, f := range favs {
		if f.Kind != models.FavoritePlayer {
			continue
		}
		name := f.ID
		if p, err := repos.GetPlayer(ctx, f.ID); err == nil {
			name = p.Name
		}
		out = append(out, core.RefreshRequest{PlayerID: f.ID, Name: name, Automatic: true})
	}
	return out
}

// newestDownloaded is the newest season refreshed for its table, or "".
// A season row from a club's matches alone doesn't count as following.
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

// internationalRefreshes follows starred nations and competitions only,
// plus a one-off national teams directory.
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

// favoriteLeagues treats an unreadable favourites table as none.
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

// currentSeasonStaleness returns a reason to show and whether it's stale.
func currentSeasonStaleness(ctx context.Context, repos storage.Repositories, compID, seasonID string,
	now time.Time, maxAge time.Duration) (string, bool) {

	run, err := repos.LatestSyncRun(ctx, compID, seasonID)
	if err != nil {
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

func previousSeason(season string) string {
	y, err := strconv.Atoi(season)
	if err != nil {
		return ""
	}
	return strconv.Itoa(y - 1)
}
