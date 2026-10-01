package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// themeScreen records ThemeChangedMsgs.
type themeScreen struct {
	fakeScreen
	modes *[]styles.Mode
}

func (s themeScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	if m, ok := msg.(core.ThemeChangedMsg); ok {
		*s.modes = append(*s.modes, m.Mode)
	}
	return s, nil
}

func TestTTogglesThemeForEveryScreen(t *testing.T) {
	root := themeScreen{fakeScreen: newFake("Browse"), modes: new([]styles.Mode)}
	top := themeScreen{fakeScreen: newFake("Arsenal"), modes: new([]styles.Mode)}
	appearance := &styles.Appearance{}
	app := NewApp(core.Deps{Theme: styles.Default(), Appearance: appearance}, root)
	app, _ = drive(t, app, core.Push(top))

	app, _ = drive(t, app, press("t"))
	if appearance.Mode != styles.Strong {
		t.Fatalf("mode is %v after t, want strong", appearance.Mode)
	}
	for _, s := range []themeScreen{root, top} {
		if got := *s.modes; len(got) != 1 || got[0] != styles.Strong {
			t.Errorf("%s heard %v", s.name, got)
		}
	}
	if !strings.Contains(app.View(), "theme: strong") {
		t.Error("the switch should be confirmed on the status line")
	}

	drive(t, app, press("t"))
	if appearance.Mode != styles.Weak {
		t.Error("a second t should switch back to weak")
	}
}

func TestTIsTypedIntoTextBoxesAndExplainedWithoutColor(t *testing.T) {
	browse := newFake("Browse")
	browse.capturing = true
	app := NewApp(core.Deps{Theme: styles.Default(), Appearance: &styles.Appearance{}}, browse)
	drive(t, app, press("t"))
	if got := *browse.keys; len(got) != 1 || got[0] != "t" {
		t.Errorf("t should reach the search box, screen got %v", got)
	}

	plain := newApp(newFake("Team"))
	plain, _ = drive(t, plain, press("t"))
	if !strings.Contains(plain.View(), "colors are off") {
		t.Errorf("without color the toggle should explain itself, got %q", plain.View())
	}
}
