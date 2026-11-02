package cli

import (
	"context"
	"fmt"
	gosync "sync"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/textfold"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// runClub fetches a club's due understat league seasons, then its ESPN
// units, then its last game's detail. Zero staleAfter fetches whatever has
// changed. lastGame says what the last step fetched, "" for nothing.
func runClub(ctx context.Context, store storage.Repositories, club models.Team, staleAfter time.Duration,
	ev refreshEvents) (results []sync.UnitResult, lastGame string, err error) {

	plan, err := sync.PlanClub(ctx, store, club, time.Now(), staleAfter)
	if err != nil {
		return nil, "", err
	}
	if len(plan.Leagues) > 0 {
		// The plan already decided these are due; MaxAge would only skip a
		// season cached recently but before club matches were.
		results = runRefresh(ctx, store, plan.Leagues, 0, ev)
		if rateLimited(results) {
			return results, "", nil
		}
	}
	ms := newMatchSyncer(store, ev)
	ms.StaleAfter = staleAfter
	results = append(results, ms.SyncUnits(ctx, plan.Units, ev.unitFunc())...)
	if rateLimited(results) {
		return results, "", nil
	}
	return results, refreshLastGame(ctx, store, club, ev), nil
}

// refreshLastGame fetches the club's last game in full. Its failure is
// noted, never the refresh's: the games themselves are already stored.
func refreshLastGame(ctx context.Context, store storage.Repositories, club models.Team, ev refreshEvents) string {
	if lastGameFailedRecently(club.ID, time.Now()) {
		return ""
	}
	res, err := sync.RefreshLastGame(ctx, store, newLastGameSource(ev), club.ID, time.Now())
	note := func(s string) {
		if ev.note != nil {
			ev.note(s)
		}
	}
	switch {
	case err != nil:
		lastGameFailed(club.ID, time.Now())
		note("last game: " + tuiError(err).Error())
		return ""
	case res.Unavailable:
		note("last game: no detail available")
		return ""
	case !res.Fetched:
		return ""
	}
	note("last game: timeline, stats and lineups fetched")
	return "last game in full"
}

// clubDue applies runClub's rules without making a request.
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
	if _, due := sync.LastGameDue(ctx, repos, club.ID); due && !lastGameFailedRecently(club.ID, now) {
		return "last game", true
	}
	return "", false
}

// lastGameBackoff is how long a club's failed last-game fetch rests before
// it is due again, as a failed unit does. It is kept in memory: a restart
// may try once more.
const lastGameBackoff = 10 * time.Minute

var lastGameFailures = struct {
	mu gosync.Mutex
	at map[string]time.Time
}{at: map[string]time.Time{}}

func lastGameFailed(teamID string, now time.Time) {
	lastGameFailures.mu.Lock()
	defer lastGameFailures.mu.Unlock()
	lastGameFailures.at[teamID] = now
}

func lastGameFailedRecently(teamID string, now time.Time) bool {
	lastGameFailures.mu.Lock()
	defer lastGameFailures.mu.Unlock()
	at, ok := lastGameFailures.at[teamID]
	return ok && now.Sub(at) < lastGameBackoff
}

// clubRefreshes covers starred clubs' cups and European games; their
// league tables are followed by the competition rule.
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

// exactClub finds the one cached club a query names exactly; national
// teams are left to the nation lookup.
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
