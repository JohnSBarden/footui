package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestRequestedRefreshesRunInOrderAndEveryScreenHears(t *testing.T) {
	screen := newFake("Catalog")
	r := &fakeRefresher{summary: "ok"}
	app := newAppWithRefresh(screen, r)

	app, _ = drive(t, app, core.RequestRefresh(
		core.RefreshRequest{CompetitionID: "eng-premier-league", Seasons: []string{"2025", "2024"}},
		core.RefreshRequest{CompetitionID: "esp-la-liga", Seasons: []string{"2020"}},
	))

	if want := []string{"eng-premier-league", "esp-la-liga"}; !slices.Equal(r.called, want) {
		t.Fatalf("ran %v, want %v in order", r.called, want)
	}
	if got := r.requests[0].Seasons; !slices.Equal(got, []string{"2025", "2024"}) {
		t.Errorf("seasons passed through as %v", got)
	}
	if got := *screen.refreshed; !slices.Equal(got, []string{"eng-premier-league", "esp-la-liga"}) {
		t.Errorf("screen heard %v", got)
	}
	if got := app.View(); !strings.Contains(got, "Refreshed 2 competitions") {
		t.Errorf("a batch should be summed up once it drains, got %q", got)
	}
	if app.active != nil || len(app.queue) != 0 {
		t.Error("queue should be idle afterwards")
	}
}

// A club's recent games already being fetched aren't queued again.
func TestRecentGamesAlreadyRunningAreNotQueuedAgain(t *testing.T) {
	app := newAppWithRefresh(newFake("Team"), &fakeRefresher{summary: "ok"})
	app.active = &core.RefreshRequest{TeamID: "eng-arsenal", Automatic: true}
	app.enqueue(core.RefreshRequest{TeamID: "eng-arsenal", Automatic: true, Why: "opened"},
		core.RefreshRequest{TeamID: "eng-chelsea", Automatic: true})
	if len(app.queue) != 1 || app.queue[0].TeamID != "eng-chelsea" {
		t.Errorf("queue %+v", app.queue)
	}
}

// One league failing must neither stop the rest of the queue nor be
// hidden by a later league succeeding.
func TestABatchFailureIsReportedAfterTheRestRun(t *testing.T) {
	r := &fakeRefresher{summary: "ok", errFor: map[string]error{"eng-premier-league": errors.New("timed out")}}
	app := newAppWithRefresh(newFake("Catalog"), r)

	app, _ = drive(t, app, core.RequestRefresh(
		core.RefreshRequest{CompetitionID: "eng-premier-league", Seasons: []string{"2025"}},
		core.RefreshRequest{CompetitionID: "esp-la-liga", Seasons: []string{"2025"}},
	))

	if len(r.called) != 2 {
		t.Fatalf("ran %v; a failure must not abandon the queue", r.called)
	}
	got := app.View()
	if !strings.Contains(got, "refreshed 1 of 2") || !strings.Contains(got, "eng-premier-league (timed out)") {
		t.Errorf("the failure should survive the later success, got %q", got)
	}
}

func TestQueuedSeasonsOfOneLeagueAreMerged(t *testing.T) {
	q := mergeRequest(nil, core.RefreshRequest{CompetitionID: "a", Seasons: []string{"2020"}})
	q = mergeRequest(q, core.RefreshRequest{CompetitionID: "b", Seasons: []string{"2020"}})
	q = mergeRequest(q, core.RefreshRequest{CompetitionID: "a", Seasons: []string{"2022", "2020"}})

	if len(q) != 2 {
		t.Fatalf("got %d requests, want 2", len(q))
	}
	if !slices.Equal(q[0].Seasons, []string{"2022", "2020"}) {
		t.Errorf("merged seasons %v, want newest first without duplicates", q[0].Seasons)
	}
}

func TestStartupRefreshRunsOnInitAndExplainsItself(t *testing.T) {
	screen := newFake("Browse")
	r := &fakeRefresher{summary: "2026-27 — 20 teams"}
	app := newAppWithRefresh(screen, r)
	app.QueueOnStart(core.RefreshRequest{CompetitionID: "eng-premier-league",
		Seasons: []string{"2026"}, Why: "cached 13 days ago"})

	cmd := app.Init()
	if !strings.Contains(app.View(), "cached 13 days ago") {
		t.Errorf("an unrequested refresh should say why it's running, got %q", app.View())
	}
	drive(t, app, cmd)

	if len(r.called) != 1 {
		t.Fatalf("startup refresh ran %d times", len(r.called))
	}
	if !*screen.initialized {
		t.Error("the screen must still be initialized")
	}
	if got := *screen.refreshed; len(got) != 1 {
		t.Errorf("screen should reload after the startup refresh, heard %v", got)
	}
}

func TestStartupRefreshIsSkippedWithoutARefresher(t *testing.T) {
	app := newApp(newFake("Browse"))
	app.QueueOnStart(core.RefreshRequest{CompetitionID: "eng-premier-league"})
	app.Init()
	if app.active != nil {
		t.Error("nothing can run without a refresher")
	}
}

func press(name string) tea.Cmd {
	return func() tea.Msg { return keyPress(name) }
}

func catalogDeps(r *fakeRefresher) core.Deps {
	deps := core.Deps{
		Theme:   styles.Plain(),
		Catalog: []core.CatalogEntry{{Competition: models.Competition{ID: "eng-premier-league"}, Seasons: []string{"2026"}}},
	}
	if r != nil {
		deps.Refresh = r.fn()
	}
	deps.Nav.Catalog = func(core.Deps) core.Screen { return newFake("Download leagues") }
	return deps
}

func TestDOpensTheCatalogOnceAndCtrlGWorksWhileTyping(t *testing.T) {
	browse := newFake("Browse")
	browse.capturing = true
	app := NewApp(catalogDeps(nil), browse)

	if footer := app.footer(); !strings.Contains(footer, "ctrl+g") {
		t.Errorf("the footer should offer ctrl+g while typing, got %q", footer)
	}

	app, _ = drive(t, app, press("d"))
	if len(app.stack) != 1 {
		t.Fatal("d must reach a focused text box, not open the catalog")
	}

	app, _ = drive(t, app, press("ctrl+g"))
	if app.current().Title() != "Download leagues" {
		t.Fatalf("ctrl+g should open the catalog, on top is %q", app.current().Title())
	}

	app, _ = drive(t, app, press("d"))
	if len(app.stack) != 2 {
		t.Errorf("d on the catalog must not stack a second one, stack is %d deep", len(app.stack))
	}
}
