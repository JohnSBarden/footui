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

// Opening a player is something being looked at now: their request goes
// ahead of background league refreshes, and asking twice is asking once.
func TestPlayerRequestJumpsTheQueueOnce(t *testing.T) {
	app := newAppWithRefresh(newFake("Dashboard"), &fakeRefresher{summary: "ok"})
	app.active = &core.RefreshRequest{CompetitionID: "eng-premier-league"}
	app.queue = []core.RefreshRequest{{CompetitionID: "esp-la-liga"}, {CompetitionID: "ita-serie-a"}}
	raya := core.RefreshRequest{PlayerID: "p1", Name: "David Raya", Automatic: true}
	app.enqueue(raya)
	app.enqueue(raya)
	if len(app.queue) != 3 || app.queue[0].PlayerID != "p1" {
		t.Fatalf("queue = %+v, want Raya first and once", app.queue)
	}
	app.active = &raya
	app.queue = nil
	app.enqueue(raya)
	if len(app.queue) != 0 {
		t.Errorf("a player already refreshing isn't queued again: %+v", app.queue)
	}
}

// r on a player asks for their whole history. Opening them already
// queued (or started) their detail, which doesn't answer that; and a
// later automatic request doesn't water a waiting history request down.
func TestHistoryRequestIsNotSwallowedByTheDetail(t *testing.T) {
	app := newAppWithRefresh(newFake("Dashboard"), &fakeRefresher{summary: "ok"})
	detail := core.RefreshRequest{PlayerID: "p1", Name: "Antoine Griezmann", Automatic: true}
	history := core.RefreshRequest{PlayerID: "p1", Name: "Antoine Griezmann", History: true}

	app.active = &detail
	app.enqueue(history)
	if len(app.queue) != 1 || !app.queue[0].History {
		t.Fatalf("queue = %+v, want the history waiting", app.queue)
	}
	app.enqueue(detail)
	if len(app.queue) != 1 || !app.queue[0].History || app.queue[0].Automatic {
		t.Errorf("queue = %+v, want one manual history request", app.queue)
	}

	club := core.RefreshRequest{TeamID: "t1", Name: "Orlando City SC", Automatic: true, Why: "recent games"}
	app.queue = []core.RefreshRequest{club}
	app.enqueue(core.RefreshRequest{TeamID: "t1", Name: "Orlando City SC", History: true})
	if len(app.queue) != 1 || !app.queue[0].History || app.queue[0].Automatic || app.queue[0].Why != "" {
		t.Errorf("queue = %+v, want the club's recent games upgraded to its history", app.queue)
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
