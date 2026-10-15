package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/textfold"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// runClub fetches what a club's recent games need: its understat league
// seasons that are due (one request each, through the club syncer), then
// the ESPN units it plays in. staleAfter holds the current season and
// year to the stale threshold; zero fetches whatever has changed.
func runClub(ctx context.Context, store storage.Repositories, club models.Team, staleAfter time.Duration,
	ev refreshEvents) ([]sync.UnitResult, error) {

	plan, err := sync.PlanClub(ctx, store, club, time.Now(), staleAfter)
	if err != nil {
		return nil, err
	}
	var results []sync.UnitResult
	if len(plan.Leagues) > 0 {
		// The plan already decided these are due; MaxAge would only skip a
		// season cached recently but before club matches were.
		results = runRefresh(ctx, store, plan.Leagues, 0, ev)
		if rateLimited(results) {
			return results, nil
		}
	}
	ms := newMatchSyncer(store, ev)
	ms.StaleAfter = staleAfter
	return append(results, ms.SyncUnits(ctx, plan.Units, ev.unitFunc())...), nil
}

// clubDue reports whether a club's recent games need anything fetched,
// and why, by the same rules runClub fetches by — so asking costs no
// request, and a club whose games are current asks nothing of anyone.
func clubDue(ctx context.Context, repos storage.Repositories, club models.Team, now time.Time,
	staleAfter time.Duration) (string, bool) {

	if club.National {
		return "", false
	}
	if at, err := repos.LastRateLimited(ctx); err == nil && now.Sub(at) < rateLimitPause {
		return "", false
	}
	plan, err := sync.PlanClub(ctx, repos, club, now, staleAfter)
	if err != nil {
		return "", false
	}
	if len(plan.Leagues) > 0 {
		return "recent league games", true
	}
	for _, u := range plan.Units {
		if why, due := sync.UnitDue(ctx, repos, u, now, staleAfter); due {
			return fmt.Sprintf("%s %d %s", u.Competition.Abbrev(), u.Year, why), true
		}
	}
	return "", false
}

// clubRefreshes is the stale check for starred clubs' recent games: one
// request per club with anything due. Their league tables are followed by
// the competition rule; this covers their cups and European games.
func clubRefreshes(ctx context.Context, repos storage.Repositories, now time.Time, maxAge time.Duration) []core.RefreshRequest {
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		return nil
	}
	var out []core.RefreshRequest
	for _, f := range favs {
		if f.Kind != models.FavoriteTeam {
			continue
		}
		club, err := repos.GetTeam(ctx, f.ID)
		if err != nil || club.National {
			continue
		}
		if why, due := clubDue(ctx, repos, club, now, maxAge); due {
			out = append(out, core.RefreshRequest{TeamID: club.ID, Name: club.Name, Why: why, Automatic: true})
		}
	}
	return out
}

// exactClub finds the one cached club a query names exactly, so `footy
// refresh --team arsenal` means Arsenal. National teams are left to the
// nation lookup.
func exactClub(ctx context.Context, repos storage.Repositories, query string) (models.Team, bool) {
	teams, err := repos.FindTeamsByName(ctx, query)
	if err != nil {
		return models.Team{}, false
	}
	want := textfold.Fold(query)
	var found []models.Team
	for _, t := range teams {
		if !t.National && (textfold.Fold(t.Name) == want || textfold.Fold(t.ShortName) == want) {
			found = append(found, t)
		}
	}
	if len(found) != 1 {
		return models.Team{}, false
	}
	return found[0], true
}
