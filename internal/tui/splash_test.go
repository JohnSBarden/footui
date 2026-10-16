package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

func TestSplashListsWhatIsRefreshing(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	app.active = &core.RefreshRequest{CompetitionID: "eng-premier-league", Name: "Premier League"}
	app.queue = []core.RefreshRequest{{CompetitionID: "ita-serie-a", Name: "Serie A"}}
	app.splash.done = []splashDone{{label: "NWSL"}}
	v := ansi.Strip(app.View())
	for _, want := range []string{"● footui", "Refreshing 3 leagues…", "✓ NWSL", "◐ Premier League", "◌ Serie A"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q\n%s", want, v)
		}
	}
}

func TestSplashWithNothingQueuedIsJustTheWordmark(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	v := ansi.Strip(app.View())
	if !strings.Contains(v, "● footui") || strings.Contains(v, "Refreshing") || strings.Contains(v, "view:Dashboard") {
		t.Errorf("idle splash\n%s", v)
	}
}

func TestAnyKeySkipsTheSplashWithoutReachingTheScreen(t *testing.T) {
	screen := newFake("Dashboard")
	app := newApp(screen)
	app.ShowSplash(false)
	app = send(app, keyPress("x"))
	if app.splash != nil || !strings.Contains(app.View(), "view:Dashboard") {
		t.Errorf("a key should dismiss the splash\n%s", app.View())
	}
	if len(*screen.keys) != 0 {
		t.Errorf("the dismissing key reached the screen: %v", *screen.keys)
	}
}

func TestTheSplashTimesOut(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	app = send(app, splashDoneMsg{})
	if app.splash != nil {
		t.Error("the timer should dismiss the splash")
	}
}

func TestSplashShowsTheESPNPause(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.deps.ProviderPause = func() (string, time.Time) { return "ESPN", time.Now().Add(47 * time.Minute) }
	app.ShowSplash(false)
	if v := ansi.Strip(app.View()); !strings.Contains(v, "✕ ESPN paused requests: retrying in 4") {
		t.Errorf("pause missing\n%s", v)
	}
}

func TestSplashWaitsForFirstDataOnAnEmptyDatabase(t *testing.T) {
	app := newApp(newFake("Browse"))
	app.ShowSplash(true)
	app.queue = []core.RefreshRequest{{CompetitionID: "eng-premier-league"}}
	app = send(app, splashDoneMsg{})
	if app.splash == nil {
		t.Fatal("an empty database keeps the splash until a league lands")
	}
	app.noteRefreshed(core.RefreshRequest{CompetitionID: "eng-premier-league", Name: "Premier League"}, nil)
	if app.splash != nil {
		t.Error("the first league landing dismisses it")
	}
}

func TestAnEmptyDatabaseWithNothingQueuedDoesNotWaitForever(t *testing.T) {
	app := newApp(newFake("Browse"))
	app.ShowSplash(true)
	app = send(app, splashDoneMsg{})
	if app.splash != nil {
		t.Error("with nothing coming, the splash must time out anyway")
	}
}
