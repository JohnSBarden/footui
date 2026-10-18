package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestSplashListsWhatIsRefreshing(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	app.active = &core.RefreshRequest{CompetitionID: "eng-premier-league", Name: "Premier League"}
	app.queue = []core.RefreshRequest{{CompetitionID: "ita-serie-a", Name: "Serie A"}}
	app.splash.done = []splashDone{{label: "NWSL"}}
	v := ansi.Strip(app.View())
	for _, want := range []string{"football stats in your terminal", "✓ NWSL", "up to date", "⠋ Premier League", "refreshing", "· Serie A", "50%", "syncing local cache…"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q\n%s", want, v)
		}
	}
}

func TestSplashSaysWhenTheRefreshIsFinished(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(true)
	app.splash.done = []splashDone{{label: "NWSL"}, {label: "MLS", failed: true}}
	v := ansi.Strip(app.View())
	for _, want := range []string{"✕ MLS", "failed", "100%", "ready · 1 failed"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q\n%s", want, v)
		}
	}
}

func TestSplashWithNothingQueuedIsJustTheWordmark(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.deps.Version = "1.0.0"
	app.ShowSplash(false)
	v := ansi.Strip(app.View())
	if !strings.Contains(v, "football stats in your terminal · v1.0.0") || strings.Contains(v, "%") || strings.Contains(v, "view:Dashboard") {
		t.Errorf("idle splash\n%s", v)
	}
	if !strings.Contains(v, "▀▀▀▀") {
		t.Errorf("the full-size splash should draw the balls' shadows\n%s", v)
	}
}

func TestSplashTaglineVersion(t *testing.T) {
	for version, want := range map[string]string{
		"":              "football stats in your terminal",
		"1.0.0":         "football stats in your terminal · v1.0.0",
		"v1.0.0":        "football stats in your terminal · v1.0.0",
		"2cebdf5-dirty": "football stats in your terminal · 2cebdf5-dirty",
	} {
		app := newApp(newFake("Dashboard"))
		app.deps.Version = version
		if got := app.splashTagline(); got != want {
			t.Errorf("version %q: got %q, want %q", version, got, want)
		}
	}
}

func TestSplashFallsBackToTheSmallWordmarkInATinyTerminal(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	app = send(app, tea.WindowSizeMsg{Width: 40, Height: 10})
	v := ansi.Strip(app.View())
	if !strings.Contains(v, "● footui") || strings.Contains(v, "▀") {
		t.Errorf("small splash\n%s", v)
	}
	for i, line := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line %d is %d wide in a 40-column terminal", i, w)
		}
	}
}

func TestTheSplashBallsBounceAndThenSettle(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	first := app.View()
	app = send(app, splashFrameMsg{})
	if app.View() == first {
		t.Error("a frame should move the balls")
	}
	for range int(2 * time.Second / splashFrameEvery) {
		app = send(app, splashFrameMsg{})
	}
	if b := app.splash.balls; b.amp != 0 || b.spin != 0 {
		t.Errorf("with nothing to refresh the balls should come to rest: %+v", b)
	}
}

func TestTheSplashBallsKeepBouncingWhileRefreshing(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.ShowSplash(false)
	app.active = &core.RefreshRequest{CompetitionID: "eng-premier-league"}
	for range int(2 * time.Second / splashFrameEvery) {
		app = send(app, splashFrameMsg{})
	}
	if app.splash.balls.amp != 1 {
		t.Errorf("the balls settled mid-refresh: %+v", app.splash.balls)
	}
}

func TestSplashFramesStopOnceItHasGone(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	if cmd := app.splashAnimate(); cmd != nil {
		t.Error("no more frames once the splash is gone")
	}
}

func TestPentagonsAreHolesWithoutColor(t *testing.T) {
	panels := 0
	for _, row := range renderBall(0) {
		for _, c := range row {
			if c == cellPanel {
				panels++
			}
		}
	}
	if panels == 0 {
		t.Fatal("the ball should show some pentagons")
	}
	plain := ansi.Strip(splashArt(styles.Plain(), newSplashBalls()))
	colored := ansi.Strip(splashArt(styles.New(styles.DefaultPalette()), newSplashBalls()))
	if strings.Count(plain, "█") >= strings.Count(colored, "█") {
		t.Error("without color, pentagons should be left blank rather than drawn solid")
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
