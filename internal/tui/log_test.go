package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/trace"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Every refresh is written to the log: queued, started, its progress, and
// how it ended.
func TestRefreshesAreLogged(t *testing.T) {
	r := &fakeRefresher{progress: []string{"season 2025"}, summary: "20 teams",
		errFor: map[string]error{"esp-la-liga": errors.New("timed out")}}
	log := core.NewLog(nil)
	app := NewApp(core.Deps{Theme: styles.Plain(), Refresh: r.fn(), Log: log}, newFake("Catalog"))

	drive(t, app, core.RequestRefresh(
		core.RefreshRequest{CompetitionID: "eng-premier-league", Seasons: []string{"2025"}},
		core.RefreshRequest{CompetitionID: "esp-la-liga", Why: "cached 21h ago"},
	))

	var got []string
	for _, e := range log.Entries() {
		got = append(got, e.Level.String()+" "+e.Text)
	}
	want := []string{
		"INFO queue eng-premier-league (2025)",
		"INFO queue esp-la-liga — cached 21h ago",
		"INFO start eng-premier-league (2025)",
		"PROG season 2025",
		"DONE eng-premier-league: 20 teams",
		"INFO start esp-la-liga — cached 21h ago",
		"PROG season 2025",
		"FAIL esp-la-liga: timed out",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// animatedScreen is a fake screen that asks for animation frames.
type animatedScreen struct {
	fakeScreen
	frames *int
}

func (a animatedScreen) Animating() bool { return true }

func (a animatedScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	if _, ok := msg.(core.FrameMsg); ok {
		*a.frames++
	}
	return a, nil
}

// Frames are asked for only while the screen on top animates, one at a
// time.
func TestFramesTickOnlyForAnAnimatedScreen(t *testing.T) {
	still := NewApp(core.Deps{Theme: styles.Plain()}, newFake("Browse"))
	if _, cmd := still.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); cmd != nil {
		if msg := cmd(); msg != nil {
			if _, ok := msg.(core.FrameMsg); ok {
				t.Error("a still screen shouldn't get frames")
			}
		}
	}

	screen := animatedScreen{fakeScreen: newFake("Dashboard"), frames: new(int)}
	app := NewApp(core.Deps{Theme: styles.Plain()}, screen)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil || !app.framing {
		t.Fatal("an animated screen should have a frame on its way")
	}
	if _, again := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); again != nil && again() != nil {
		t.Error("a second frame shouldn't be asked for while one is pending")
	}
	app.Update(core.FrameMsg{})
	if *screen.frames != 1 || !app.framing {
		t.Errorf("frames %d, framing %v: a frame should reach the screen and ask for the next", *screen.frames, app.framing)
	}
}

// L opens the log beside any screen, which is narrowed to make room; on a
// narrow terminal the panel takes the screen's place.
func TestLogPanelOpensBesideAnyScreen(t *testing.T) {
	log := core.NewLog(nil)
	log.Add(core.LogDone, "MLS: 30 teams")
	log.Add(core.LogInfo, "start Premier League (2025)")
	log.Add(core.LogProgress, "season 2025: 380 matches")
	screen := newFake("Team")
	app := NewApp(core.Deps{Theme: styles.Plain(), Log: log}, screen)
	app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app.active = &core.RefreshRequest{CompetitionID: "eng-premier-league", Name: "Premier League"}
	app.queue = []core.RefreshRequest{{CompetitionID: "usa-nwsl", Name: "NWSL"}}

	app.Update(keyPress("L"))
	if *screen.width != 140-46-2 {
		t.Errorf("screen width %d with the log open, want it narrowed", *screen.width)
	}
	view := app.View()
	for _, want := range []string{"view:Team", "NOW", "🔄 Premier League", "season 2025: 380 matches",
		"QUEUED 1", "· NWSL", "RECENT", "✓ MLS: 30 teams"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if strings.Index(view, "season 2025") > strings.Index(view, "MLS: 30") {
		t.Error("recent entries should run newest first")
	}
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 140 {
			t.Errorf("line %d wide, past the screen: %q", w, line)
		}
	}

	app.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	if view := app.View(); strings.Contains(view, "view:Team") || !strings.Contains(view, "RECENT") {
		t.Errorf("a narrow terminal should show only the panel:\n%s", view)
	}

	app.Update(keyPress("L"))
	if *screen.width != 60 || strings.Contains(app.View(), "RECENT") {
		t.Error("L again should close the log and give the screen its width back")
	}
}

// The dashboard's button is a click on the "log" zone, which the app
// handles itself.
func TestLogButtonClickTogglesThePanel(t *testing.T) {
	screen := newFake("Matchday")
	app := NewApp(core.Deps{Theme: styles.Plain(), Log: core.NewLog(nil)}, screen)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app.click("log")
	view := app.View()
	for _, want := range []string{"idle — nothing downloading", "nothing yet this session"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	if !*screen.logOpen {
		t.Error("the screen should be told the log is open, for its button")
	}
}

// With debug mode on, lines from below the interface reach the panel, and
// everything reaches the file once.
func TestDebugLinesReachTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	if err := trace.Start(path); err != nil {
		t.Fatal(err)
	}
	defer trace.Stop()
	log := core.NewLog(nil)
	log.FollowTrace()
	trace.Printf("GET https://example.test → 200")
	log.Add(core.LogDone, "MLS: 30 teams")

	var got []string
	for _, e := range log.Entries() {
		got = append(got, e.Level.String()+" "+e.Text)
	}
	if strings.Join(got, "|") != "DBUG GET https://example.test → 200|DONE MLS: 30 teams" {
		t.Errorf("log %q", got)
	}
	if err := trace.Stop(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	for _, want := range []string{"DBUG GET https://example.test → 200", "DONE MLS: 30 teams"} {
		if strings.Count(string(body), want) != 1 {
			t.Errorf("debug file should hold %q once:\n%s", want, body)
		}
	}
}
