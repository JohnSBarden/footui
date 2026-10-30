package understat

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

// Griezmann's 2021-22 La Liga: one line for both clubs in the league feed.
const moverLeague = `{"teams":{},"players":[
	{"id":"2", "player_name":"Pedri", "games":"22", "time":"1700", "goals":"3", "xG":"2.5", "assists":"1", "xA":"2.0",
	 "shots":"30", "key_passes":"25", "npg":"3", "npxG":"2.5", "yellow_cards":"2", "red_cards":"0", "position":"M",
	 "team_title":"Barcelona"},
	{"id":"2254", "player_name":"Antoine Griezmann", "games":"29", "time":"1800", "goals":"3", "xG":"5.1", "assists":"6",
	 "xA":"4.0", "shots":"40", "key_passes":"30", "npg":"3", "npxG":"5.1", "yellow_cards":"4", "red_cards":"0",
	 "position":"F", "team_title":"Barcelona,Atletico Madrid"}
]}`

// His player page splits that season by club, beside other seasons.
const moverPage = `{"groups":{"season":[
	{"season":"2022", "team":"Atletico Madrid", "games":"38", "time":"2500", "goals":"15", "assists":"5", "xG":"12",
	 "xA":"5", "shots":"80", "key_passes":"50", "npg":"15", "npxG":"12", "yellow":"3", "red":"0"},
	{"season":"2021", "team":"Atletico Madrid", "games":"26", "time":"1650", "goals":"3", "assists":"6", "xG":"4.9",
	 "xA":"3.8", "shots":"38", "key_passes":"28", "npg":"3", "npxG":"4.9", "yellow":"4", "red":"0"},
	{"season":"2021", "team":"Barcelona", "games":"3", "time":"150", "goals":"0", "assists":"0", "xG":"0.2",
	 "xA":"0.2", "shots":"2", "key_passes":"2", "npg":"0", "npxG":"0.2", "yellow":"0", "red":"0"}
]}}`

func moverProvider(t *testing.T, page func(w http.ResponseWriter)) (*Provider, *int32) {
	t.Helper()
	pages := new(int32)
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getLeagueData/"):
			w.Write([]byte(moverLeague))
		case r.URL.Path == "/getPlayerData/2254":
			atomic.AddInt32(pages, 1)
			if !strings.HasSuffix(r.Header.Get("Referer"), "/player/2254") {
				t.Errorf("referer %q", r.Header.Get("Referer"))
			}
			page(w)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	return &Provider{client: c}, pages
}

func rosterOf(t *testing.T, p *Provider, label string) map[string]models.PlayerSeasonStats {
	t.Helper()
	rows, err := p.AllRosterStats(context.Background(),
		provider.CompetitionRef{CompetitionID: "esp-la-liga"},
		provider.SeasonRef{SeasonID: "esp-la-liga:" + label, Label: label})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]models.PlayerSeasonStats{}
	for _, r := range rows {
		out[r.PlayerID+" "+r.TeamID] = r
	}
	return out
}

func TestAMoverIsSplitByClub(t *testing.T) {
	p, pages := moverProvider(t, func(w http.ResponseWriter) { w.Write([]byte(moverPage)) })
	got := rosterOf(t, p, "2021")

	if len(got) != 3 {
		t.Fatalf("rows %v, want Pedri plus Griezmann at each club", got)
	}
	atleti := got["understat:2254 understat:esp-atletico-madrid"]
	barca := got["understat:2254 understat:esp-barcelona"]
	if atleti.Appearances != 26 || atleti.Minutes != 1650 || atleti.Assists != 6 || *atleti.XG != 4.9 || atleti.YellowCards != 4 {
		t.Errorf("Atlético line %+v", atleti)
	}
	if barca.Appearances != 3 || barca.Minutes != 150 || barca.Goals != 0 || *barca.Shots != 2 {
		t.Errorf("Barcelona line %+v", barca)
	}
	if barca.SeasonID != "esp-la-liga:2021" || barca.Source != "understat" {
		t.Errorf("split lines keep the season and source: %+v", barca)
	}
	if pedri := got["understat:2 understat:esp-barcelona"]; pedri.Appearances != 22 {
		t.Errorf("a one-club player is untouched: %+v", pedri)
	}

	// The page covers every season, so it's fetched once.
	rosterOf(t, p, "2022")
	if *pages != 1 {
		t.Errorf("player page fetched %d times, want once", *pages)
	}
}

// Without the page the aggregate stays, under the latest club.
func TestAMoverWithoutAPageKeepsTheAggregate(t *testing.T) {
	p, _ := moverProvider(t, func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) })
	got := rosterOf(t, p, "2021")
	line, ok := got["understat:2254 understat:esp-atletico-madrid"]
	if len(got) != 2 || !ok || line.Appearances != 29 {
		t.Errorf("rows %v, want the 29-game aggregate under Atlético", got)
	}
}
