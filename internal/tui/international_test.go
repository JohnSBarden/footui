package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Being rate-limited drops everything queued behind the refused request,
// and says why, rather than letting each queued request be refused in
// turn.
func TestARateLimitDropsTheQueue(t *testing.T) {
	r := &fakeRefresher{summary: "ok", errFor: map[string]error{"world-cup": fmt.Errorf("x: %w", core.ErrRateLimited)}}
	app := newAppWithRefresh(newFake("Catalog"), r)

	app, _ = drive(t, app, core.RequestRefresh(
		core.RefreshRequest{CompetitionID: "eng-premier-league", Seasons: []string{"2025"}},
		core.RefreshRequest{CompetitionID: "world-cup", Seasons: []string{"2022"}},
		core.RefreshRequest{CompetitionID: "euro", Seasons: []string{"2024"}},
		core.RefreshRequest{CompetitionID: "esp-la-liga", Seasons: []string{"2025"}},
	))
	if want := []string{"eng-premier-league", "world-cup"}; !slices.Equal(r.called, want) {
		t.Errorf("ran %v, want nothing after the rate limit", r.called)
	}
	if got := app.View(); !strings.Contains(got, "ESPN is rate-limiting footy; try again later") {
		t.Errorf("the status line should say why:\n%s", got)
	}
	if app.active != nil || len(app.queue) != 0 {
		t.Error("the queue should be empty and idle")
	}
}

func TestNationRequestsMergeByWhatTheyFetch(t *testing.T) {
	q := mergeRequest(nil, core.RefreshRequest{Directory: true})
	q = mergeRequest(q, core.RefreshRequest{Directory: true})
	q = mergeRequest(q, core.RefreshRequest{TeamID: "nat-england", Seasons: []string{"2026"}})
	q = mergeRequest(q, core.RefreshRequest{TeamID: "nat-england", Seasons: []string{"2025"}})
	q = mergeRequest(q, core.RefreshRequest{TeamID: "nat-spain", Seasons: []string{"2026"}})
	if len(q) != 3 {
		t.Fatalf("queue %+v, want the directory once, then England (merged) and Spain", q)
	}
	if !slices.Equal(q[1].Seasons, []string{"2026", "2025"}) {
		t.Errorf("England's years %v", q[1].Seasons)
	}
}

// Starring a nation fetches its matches (free when another nation of its
// gender already did); starring a club fetches nothing, as before.
func TestStarringANationQueuesItsMatches(t *testing.T) {
	app, repos, screen := favApp(t, false)
	r := &fakeRefresher{summary: "already up to date"}
	app.deps.Refresh = r.fn()
	if err := repos.UpsertTeam(context.Background(), models.Team{ID: "nat-arcadia", Name: "Arcadia", Gender: "men", National: true}); err != nil {
		t.Fatal(err)
	}

	pressAndSettle(app, "f") // the club
	if len(r.requests) != 0 || app.active != nil {
		t.Errorf("starring a club queued %+v", r.requests)
	}

	screen.target = core.FavoriteTarget{Kind: models.FavoriteTeam, ID: "nat-arcadia", Name: "Arcadia"}
	app.stack[0] = screen
	_, cmd := app.Update(keyPress("f"))
	drive(t, app, cmd)
	if len(r.requests) != 1 || r.requests[0].TeamID != "nat-arcadia" {
		t.Errorf("requests %+v, want Arcadia's matches", r.requests)
	}
}

func TestTheTeamRouteOpensANationForANationalTeam(t *testing.T) {
	deps := NewDeps(context.Background(), nil, styles.Plain())
	if got := deps.Nav.Team(deps, models.Team{ID: "nat-arcadia", Name: "Arcadia", National: true}, "", ""); got.Title() != "Arcadia" {
		t.Fatalf("got %T", got)
	}
	if _, ok := deps.Nav.Team(deps, models.Team{ID: "nat-arcadia", Name: "Arcadia", National: true}, "", "").(core.RefreshRequester); !ok {
		t.Error("a national team should open the nation screen")
	}
	if _, ok := NewTeamScreen(deps, models.Team{ID: "eng-arsenal", Name: "Arsenal"}).(core.RefreshRequester); ok {
		t.Error("a club should open the team screen")
	}
}

// requesterScreen hands the app a whole refresh request, as the nation
// screen does.
type requesterScreen struct {
	fakeScreen
	req core.RefreshRequest
}

func (r requesterScreen) RefreshRequest() (core.RefreshRequest, bool) { return r.req, true }

func TestTheRefreshKeyUsesAScreensOwnRequest(t *testing.T) {
	screen := requesterScreen{fakeScreen: newFake("England"), req: core.RefreshRequest{TeamID: "nat-england", Name: "England"}}
	r := &fakeRefresher{summary: "already up to date"}
	app := newAppWithRefresh(screen, r)
	if _, ok := app.refreshBinding(); !ok {
		t.Fatal("the refresh key should be offered")
	}
	app, _ = drive(t, app, app.startRefresh())
	if len(r.requests) != 1 || r.requests[0].TeamID != "nat-england" {
		t.Fatalf("requests %+v", r.requests)
	}
	if got := app.View(); !strings.Contains(got, "Refreshed England: already up to date") {
		t.Errorf("status should name the team:\n%s", got)
	}
}
