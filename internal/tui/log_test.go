package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
