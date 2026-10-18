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

// Starred players are queued with the leagues on launch; the summary
// mustn't call them competitions.
func TestBatchSummaryCountsPlayersApart(t *testing.T) {
	r := &fakeRefresher{summary: "ok"}
	app := newAppWithRefresh(newFake("Dashboard"), r)
	app, _ = drive(t, app, core.RequestRefresh(
		core.RefreshRequest{CompetitionID: "eng-premier-league", Name: "Premier League"},
		core.RefreshRequest{PlayerID: "p1", Name: "David Raya"},
		core.RefreshRequest{PlayerID: "p2", Name: "Gabriel"},
	))
	if got := app.View(); !strings.Contains(got, "Refreshed 1 competition and 2 players: Premier League, David Raya, Gabriel") {
		t.Errorf("summary:\n%s", got)
	}
}
