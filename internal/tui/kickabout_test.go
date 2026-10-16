package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

var ctrlK = tea.KeyMsg{Type: tea.KeyCtrlK}

func send(app *App, msg tea.Msg) *App {
	m, _ := app.Update(msg)
	return m.(*App)
}

func TestKickaboutRestoresTheScreenExactly(t *testing.T) {
	app := newApp(newFake("League"))
	before := app.View()
	app = send(app, ctrlK)
	if app.View() == before || !strings.Contains(app.View(), "kickabout") {
		t.Fatalf("ctrl+k should show the pitch\n%s", app.View())
	}
	app = send(app, keyPress("esc"))
	if app.View() != before {
		t.Errorf("esc should restore the screen exactly\nbefore:\n%s\nafter:\n%s", before, app.View())
	}
}

func TestKeysDuringKickaboutStayInTheGame(t *testing.T) {
	screen := newFake("League")
	app := send(newApp(screen), ctrlK)
	app = send(app, keyPress("q"))
	app = send(app, keyPress("j"))
	if len(*screen.keys) != 0 {
		t.Errorf("keys leaked to the screen: %v", *screen.keys)
	}
	if app.kick == nil {
		t.Error("q must not end the game; only esc does")
	}
	if _, cmd := app.Update(keyPress("ctrl+c")); !isQuit(cmd) {
		t.Error("ctrl+c still quits")
	}
}

func TestRefreshDuringKickaboutReachesTheScreen(t *testing.T) {
	screen := newFake("League")
	app := send(newApp(screen), ctrlK)
	app = send(app, core.RefreshedMsg{CompetitionID: "eng-premier-league"})
	app = send(app, core.StatusMsg{Text: "refreshed Premier League"})
	if len(*screen.refreshed) != 1 {
		t.Errorf("the screen beneath missed the refresh: %v", *screen.refreshed)
	}
	app = send(app, keyPress("esc"))
	if !strings.Contains(app.View(), "refreshed Premier League") {
		t.Errorf("the status set mid-game should show after\n%s", app.View())
	}
}

func TestResizeDuringKickaboutResizesBoth(t *testing.T) {
	screen := newFake("League")
	app := send(newApp(screen), ctrlK)
	app = send(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	if *screen.width != 120 {
		t.Errorf("screen width %d", *screen.width)
	}
	if w := lipgloss.Width(strings.Split(app.View(), "\n")[0]); w != 120 {
		t.Errorf("pitch width %d after resize", w)
	}
}

func TestKickaboutIsOnlyInTheFullHelp(t *testing.T) {
	app := newApp(newFake("League"))
	if strings.Contains(app.View(), "kickabout") {
		t.Error("kickabout must stay out of the footer")
	}
	app = send(app, keyPress("?"))
	if !strings.Contains(app.View(), "kickabout") {
		t.Errorf("? help should list it\n%s", app.View())
	}
}
