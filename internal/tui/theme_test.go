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
	modes  *[]styles.Mode
	themes *styles.Theme
}

func (s themeScreen) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	if m, ok := msg.(core.ThemeChangedMsg); ok {
		*s.modes = append(*s.modes, m.Mode)
		if s.themes != nil {
			*s.themes = m.Theme
		}
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

// The opening screen always has its search box focused, so "t" is a
// letter there; ctrl+t must still switch the theme.
func TestCtrlTTogglesWhileTyping(t *testing.T) {
	browse := newFake("Browse")
	browse.capturing = true
	appearance := &styles.Appearance{}
	app := NewApp(core.Deps{Theme: styles.Default(), Appearance: appearance}, browse)

	app, _ = drive(t, app, press("ctrl+t"))
	if appearance.Mode != styles.Strong {
		t.Fatal("ctrl+t should switch the theme from a screen that captures text")
	}
	if footer := app.footer(); !strings.Contains(footer, "ctrl+t") {
		t.Errorf("the footer should offer ctrl+t while typing, got %q", footer)
	}
}

// Strong mode must be visible on every screen, not only on a club's.
func TestStrongModePaintsScreensWithoutAClubTheme(t *testing.T) {
	app := NewApp(core.Deps{Theme: styles.Default(), Appearance: &styles.Appearance{}}, newFake("League"))
	if app.theme().Background != nil {
		t.Fatal("weak mode should leave the terminal background alone")
	}
	app, _ = drive(t, app, press("t"))
	if app.theme().Background == nil {
		t.Error("strong mode should paint a screen with no club colors too")
	}
	if !strings.Contains(app.footer(), "theme") {
		t.Errorf("the footer should always offer the theme key, got %q", app.footer())
	}
}

// Screens draw their own content, so they must be handed the new theme,
// or a painted background ends up under text styled for the terminal's.
func TestScreensAreHandedTheThemeForTheNewMode(t *testing.T) {
	var got styles.Theme
	screen := themeScreen{fakeScreen: newFake("League"), modes: new([]styles.Mode), themes: &got}
	app := NewApp(core.Deps{Theme: styles.Default(), Appearance: &styles.Appearance{}}, screen)
	app, _ = drive(t, app, press("t"))
	if got.Background == nil {
		t.Error("the strong base theme should reach every screen")
	}
	if app.deps.Theme.Background == nil {
		t.Error("the app's own deps should carry it too, so screens built later start strong")
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
