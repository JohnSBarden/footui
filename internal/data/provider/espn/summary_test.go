package espn

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

var faCup = provider.CompetitionRef{CompetitionID: "eng-fa-cup", Name: "FA Cup"}

// summaryServer serves the FA Cup summary and that day's scoreboard, and
// records the paths asked for.
func summaryServer(t *testing.T) (*Provider, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.RequestURI())
		var file string
		switch {
		case r.URL.Path == "/site/site/v2/sports/soccer/eng.fa/summary" && r.URL.Query().Get("event") == "4001":
			file = "summary_eng.fa_4001.json"
		case r.URL.Path == "/site/site/v2/sports/soccer/eng.fa/scoreboard":
			file = "scoreboard_eng.fa_2026.json"
		default:
			http.NotFound(w, r)
			return
		}
		body, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return newWithClient(NewClient().withBaseURLs(srv.URL+"/site", srv.URL+"/core")), &asked
}

func TestMatchDetailMapsTheSummary(t *testing.T) {
	p, asked := summaryServer(t)
	d, err := p.MatchDetail(ctxBkgnd, faCup, "4001")
	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 1 {
		t.Errorf("one request per match, got %v", *asked)
	}
	if !d.Final || d.Venue != "Harrowdene Park" || d.Attendance != 18432 || d.MatchID != "" {
		t.Errorf("header: %+v", d)
	}
	if d.Home.Formation != "4-3-3" || d.Away.Formation != "4-4-2" {
		t.Errorf("formations %q %q", d.Home.Formation, d.Away.Formation)
	}

	h, a := d.Home.Stats, d.Away.Stats
	if h.Possession == nil || *h.Possession != 58.2 || *h.Passes != 512 || *h.AccuratePasses != 441 ||
		*h.TacklesWon != 11 || *h.Clearances != 18 || *h.RedCards != 1 || *h.Corners != 8 {
		t.Errorf("home stats %+v", h)
	}
	if a.BlockedShots != nil || a.Crosses != nil || *a.Saves != 4 {
		t.Error("stats the source left out stay nil")
	}
}

func TestMatchDetailEvents(t *testing.T) {
	p, _ := summaryServer(t)
	d, err := p.MatchDetail(ctxBkgnd, faCup, "4001")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		kind, side, clock, player, other string
	}
	want := []row{
		{models.EventGoal, "home", "23'", "Tom Ashdown", "Leo Marsh"},
		{models.EventYellow, "away", "45+1'", "Owen Pryce", ""},
		{models.EventHalfTime, "", "45+3'", "", ""},
		{models.EventSub, "away", "58'", "Dan Vickers", "Owen Pryce"},
		{models.EventYellow, "home", "64'", "Leo Marsh", ""},
		{models.EventOwnGoal, "away", "71'", "Sam Kerrow", ""},
		{models.EventSecondYellow, "home", "80'", "Leo Marsh", ""},
		{models.EventPenaltyMiss, "away", "88'", "Dan Vickers", ""},
		{models.EventFullTime, "", "90+4'", "", ""},
	}
	if len(d.Events) != len(want) {
		for _, e := range d.Events {
			t.Logf("%s %s %s %s", e.Clock(), e.Kind, e.Side, e.Player)
		}
		t.Fatalf("got %d events, want %d (kick-off and delays dropped)", len(d.Events), len(want))
	}
	for i, w := range want {
		e := d.Events[i]
		got := row{e.Kind, e.Side, e.Clock(), e.Player, e.Other}
		if got != w {
			t.Errorf("event %d: got %+v, want %+v", i, got, w)
		}
	}
	if g := d.Events[0]; g.PlayerID != "espn:9101" || g.OtherID != "espn:9102" || g.Period != 1 || !g.IsGoal() {
		t.Errorf("goal %+v", g)
	}
	if og := d.Events[5]; !og.IsGoal() || og.Period != 2 {
		t.Errorf("own goal %+v", og)
	}
}

func TestMatchDetailLineups(t *testing.T) {
	p, _ := summaryServer(t)
	d, err := p.MatchDetail(ctxBkgnd, faCup, "4001")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, pl := range d.Home.Lineup {
		names = append(names, pl.Name)
	}
	if want := []string{"Ian Holt", "Sam Kerrow", "Leo Marsh", "Tom Ashdown", "Ray Colley"}; !slices.Equal(names, want) {
		t.Errorf("home lineup order %v, want starters by place then the bench %v", names, want)
	}
	keeper, marsh, bench := d.Home.Lineup[0], d.Home.Lineup[2], d.Home.Lineup[4]
	if keeper.Place != 1 || keeper.Saves != 2 || keeper.Shirt != "1" || keeper.PlayerID != "espn:9100" {
		t.Errorf("keeper %+v", keeper)
	}
	if marsh.Assists != 1 || !marsh.Yellow || !marsh.Red {
		t.Errorf("Marsh %+v", marsh)
	}
	if bench.Starter || bench.Place != 0 || bench.Played() {
		t.Errorf("unused sub %+v", bench)
	}
	off, on := d.Away.Lineup[0], d.Away.Lineup[1]
	if off.Off != 58 || off.On != 0 || on.On != 58 || !on.Played() {
		t.Errorf("sub minutes: off %+v on %+v", off, on)
	}
}

func TestMatchDetailCommentary(t *testing.T) {
	p, _ := summaryServer(t)
	d, err := p.MatchDetail(ctxBkgnd, faCup, "4001")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Commentary) != 4 {
		t.Fatalf("blank lines dropped: got %d lines", len(d.Commentary))
	}
	if c := d.Commentary[0]; c.Clock != "" || c.Kind != "" {
		t.Errorf("narration before kick-off %+v", c)
	}
	if c := d.Commentary[3]; c.Clock != "45+1'" || c.Kind != "Yellow Card" {
		t.Errorf("booking line %+v", c)
	}
}

func TestMatchDetailUnknownEventIsNotFound(t *testing.T) {
	p, _ := summaryServer(t)
	if _, err := p.MatchDetail(ctxBkgnd, faCup, "9999"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
	if _, err := p.MatchDetail(ctxBkgnd, provider.CompetitionRef{CompetitionID: "nowhere"}, "1"); err == nil {
		t.Error("an uncovered competition should fail")
	}
}

func TestDayMatchesAsksForThatDay(t *testing.T) {
	p, asked := summaryServer(t)
	day := time.Date(2026, 1, 10, 15, 0, 0, 0, time.UTC)
	matches, teams, err := p.DayMatches(ctxBkgnd, faCup, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "dates=20260110") {
		t.Errorf("asked %v", *asked)
	}
	m := matchByID(t, matches, "espn:4001")
	if m.HomeTeamID != "espn:7101" || len(teams) == 0 {
		t.Errorf("match %+v, teams %d", m, len(teams))
	}
}

func TestParseClock(t *testing.T) {
	for in, want := range map[string][2]int{
		"16'": {16, 0}, "45'+2'": {45, 2}, "90'+10'": {90, 10}, "": {0, 0}, "105'+1'": {105, 1},
	} {
		if m, a := parseClock(in); m != want[0] || a != want[1] {
			t.Errorf("parseClock(%q) = %d, %d", in, m, a)
		}
	}
}
