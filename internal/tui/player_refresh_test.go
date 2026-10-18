package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

func TestDescribeUsesRefreshEmoji(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	got := app.describe(core.RefreshRequest{PlayerID: "understat-5613", Name: "Gabriel"}, "")
	if got != "🔄 Refreshing Gabriel…" {
		t.Errorf("describe = %q", got)
	}
}

func TestRefreshedMsgCarriesThePlayer(t *testing.T) {
	msg := refreshedMsg(core.RefreshRequest{PlayerID: "understat-5613"})
	if msg.PlayerID != "understat-5613" || msg.International {
		t.Errorf("msg = %+v", msg)
	}
}

func TestSplashPauseNamesNoSource(t *testing.T) {
	app := newApp(newFake("Dashboard"))
	app.deps.ProviderPause = func() time.Time { return time.Now().Add(20 * time.Minute) }
	app.ShowSplash(false)
	v := ansi.Strip(app.View())
	if !strings.Contains(v, "refreshes paused: retrying in") {
		t.Errorf("no pause line\n%s", v)
	}
	if strings.Contains(v, "ESPN") {
		t.Errorf("splash names a source\n%s", v)
	}
}
