package espn

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

var worldCup = provider.CompetitionRef{CompetitionID: "world-cup", Name: "FIFA World Cup"}

// intlServer serves the international fixtures — hand-written in ESPN's
// shape, with fictional nations — and counts requests.
func intlServer(t *testing.T) (*Provider, *int32) {
	t.Helper()
	var requests int32
	routes := map[string]string{
		"/site/site/v2/sports/soccer/fifa.world/scoreboard": "scoreboard_fifa.world_2030.json",
		"/site/v2/sports/soccer/fifa.world/standings":       "standings_fifa.world_2030.json",
		"/site/site/v2/sports/soccer/fifa.friendly/teams":   "teams_fifa.friendly.json",
		"/site/site/v2/sports/soccer/fifa.friendly.w/teams": "teams_fifa.friendly.w.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		file, ok := routes[r.URL.Path]
		if !ok {
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
	return newWithClient(NewClient().withBaseURLs(srv.URL+"/site", srv.URL+"/core")), &requests
}

func matchByID(t *testing.T, ms []models.Match, id string) models.Match {
	t.Helper()
	for _, m := range ms {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("no match %s", id)
	return models.Match{}
}

func TestCompetitionMatchesMapsAYearInOneRequest(t *testing.T) {
	p, requests := intlServer(t)
	matches, teams, err := p.CompetitionMatches(ctxBkgnd, worldCup, 2030)
	if err != nil {
		t.Fatal(err)
	}
	if *requests != 1 {
		t.Errorf("%d requests for one competition-year, want 1", *requests)
	}
	if len(matches) != 7 || len(teams) != 6 {
		t.Fatalf("%d matches, %d teams", len(matches), len(teams))
	}
	for _, team := range teams {
		if !team.National || team.Gender != "men" || !strings.HasPrefix(team.ID, "espn:") {
			t.Errorf("team %+v", team)
		}
	}

	opener := matchByID(t, matches, "espn:3001")
	if opener.SeasonID != "world-cup:2030" || opener.Status != models.MatchFullTime || opener.Round != "Group A" {
		t.Errorf("opener %+v", opener)
	}
	if opener.HomeTeamID != "espn:9101" || *opener.HomeScore != 3 || *opener.AwayScore != 1 || opener.Neutral {
		t.Errorf("opener result %+v: a home side at home isn't neutral", opener)
	}
	hs := opener.HomeStats
	if hs == nil || *hs.Possession != 61.5 || *hs.Shots != 18 || *hs.ShotsOnTarget != 7 || *hs.Corners != 6 || *hs.Fouls != 9 {
		t.Errorf("home stats %+v", hs)
	}
	if opener.Kickoff.Hour() != 19 || opener.Kickoff.Location().String() != "UTC" {
		t.Errorf("kickoff %v", opener.Kickoff)
	}

	qf := matchByID(t, matches, "espn:3003")
	if qf.Status != models.MatchPens || *qf.HomePens != 4 || *qf.AwayPens != 5 || qf.Round != "Quarterfinal" || !qf.Neutral {
		t.Errorf("quarterfinal %+v", qf)
	}
	if sf := matchByID(t, matches, "espn:3004"); sf.Status != models.MatchAET || sf.Round != "Semifinal" || sf.HomePens != nil {
		t.Errorf("semifinal %+v", sf)
	}
	final := matchByID(t, matches, "espn:3005")
	if final.Status != models.MatchScheduled || final.HomeScore != nil || final.Round != "Final" {
		t.Errorf("an unplayed final has no score: %+v", final)
	}
	if c := matchByID(t, matches, "espn:3006"); c.Status != models.MatchCancelled || c.HomeScore != nil {
		t.Errorf("cancelled %+v", c)
	}
	// Played in January 2030, filed under the 2029 edition: the season is
	// the edition, not the year fetched.
	if late := matchByID(t, matches, "espn:3007"); late.SeasonID != "world-cup:2029" {
		t.Errorf("edition %s, want world-cup:2029", late.SeasonID)
	}
}

// 0% possession is unknown, more shots on target than shots is
// impossible, and a side reporting nothing but zeroes reported nothing.
func TestMatchStatsDropTheImplausible(t *testing.T) {
	p, _ := intlServer(t)
	matches, _, _ := p.CompetitionMatches(ctxBkgnd, worldCup, 2030)
	m := matchByID(t, matches, "espn:3002")
	if m.Round != "Group B" {
		t.Errorf("round %q: ESPN's long group names are trimmed to the group", m.Round)
	}
	if !m.Neutral {
		t.Error("a match in a third country is on neutral ground")
	}
	hs := m.HomeStats
	if hs == nil || hs.Possession != nil || hs.ShotsOnTarget != nil || *hs.Shots != 4 || *hs.Corners != 3 {
		t.Errorf("home stats %+v", hs)
	}
	if m.AwayStats != nil {
		t.Errorf("all-zero stats should be no stats, got %+v", m.AwayStats)
	}
}

// ESPN truncates a scoreboard at its limit without saying so. A response
// exactly that full is refetched month by month; a month that is full
// too means the year can't be fetched whole, and says so.
func TestAFullScoreboardIsRefetchedMonthByMonth(t *testing.T) {
	event := func(id int, month int) string {
		return fmt.Sprintf(`{"id":"%d","date":"2030-%02d-10T19:00Z","season":{"year":2030},"competitions":[{"status":{"type":{"name":"STATUS_FULL_TIME"}},`+
			`"competitors":[{"homeAway":"home","score":"1","team":{"id":"1","displayName":"Arcadia"}},{"homeAway":"away","score":"0","team":{"id":"2","displayName":"Borduria"}}]}]}`, id, month)
	}
	for _, tc := range []struct {
		name      string
		fullMonth string
		wantErr   bool
	}{{"months fit", "", false}, {"a month is full too", "203003", true}} {
		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dates := r.URL.Query().Get("dates")
			paths = append(paths, dates)
			var evs []string
			switch {
			case dates == "2030" || dates == tc.fullMonth:
				evs = []string{event(1, 1), event(2, 3), event(3, 3)}
			case dates == "203001":
				evs = []string{event(1, 1)}
			case dates == "203003":
				evs = []string{event(2, 3), event(3, 3)}
			}
			fmt.Fprintf(w, `{"events":[%s]}`, strings.Join(evs, ","))
		}))
		p := newWithClient(NewClient().withBaseURLs(srv.URL, srv.URL))
		p.limit = 3
		matches, _, err := p.CompetitionMatches(ctxBkgnd, worldCup, 2030)
		srv.Close()
		if len(paths) != 13 {
			t.Errorf("%s: %d requests, want the year then 12 months", tc.name, len(paths))
		}
		if len(matches) != 3 {
			t.Errorf("%s: %d matches, want the 3, each once", tc.name, len(matches))
		}
		if got := errors.Is(err, provider.ErrIncomplete); got != tc.wantErr {
			t.Errorf("%s: err %v", tc.name, err)
		}
	}
}

func TestNationalTeamsComeFromTheFriendliesDirectory(t *testing.T) {
	p, _ := intlServer(t)
	men, err := p.NationalTeams(ctxBkgnd, "men")
	if err != nil {
		t.Fatal(err)
	}
	if len(men) != 3 || men[0].ID != "espn:9101" || men[0].Name != "Arcadia" || !men[0].National || men[0].Gender != "men" {
		t.Errorf("men %+v", men)
	}
	women, err := p.NationalTeams(ctxBkgnd, "women")
	if err != nil {
		t.Fatal(err)
	}
	if len(women) != 2 || women[0].Name != "Arcadia" || women[0].Gender != "women" {
		t.Errorf("women %+v: the team carries the gender, so the name is the country's", women)
	}
	if _, err := p.NationalTeams(ctxBkgnd, "mixed"); err == nil {
		t.Error("an unknown gender should be an error")
	}
}

// A tournament's table keeps its groups, each ranked within itself, and
// costs one request: there are no rosters to fetch for national teams.
func TestTournamentStandingsKeepGroups(t *testing.T) {
	p, requests := intlServer(t)
	rows, err := p.Standings(ctxBkgnd, worldCup, provider.SeasonRef{SeasonID: "world-cup:2030", Label: "2030"})
	if err != nil {
		t.Fatal(err)
	}
	if *requests != 1 {
		t.Errorf("%d requests, want just the standings", *requests)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.TeamID] = fmt.Sprintf("%s/%d", r.GroupLabel, *r.Position)
	}
	want := map[string]string{"espn:9101": "Group A/1", "espn:9102": "Group A/2", "espn:9103": "Group B/1", "espn:9104": "Group B/2"}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: %s, want %s", id, got[id], w)
		}
	}
	teams, _ := p.Teams(ctxBkgnd, worldCup, provider.SeasonRef{SeasonID: "world-cup:2030", Label: "2030"})
	if len(teams) != 4 || !teams[0].National {
		t.Errorf("teams %+v", teams)
	}
	if players, _ := p.Players(ctxBkgnd, worldCup, provider.SeasonRef{SeasonID: "world-cup:2030", Label: "2030"}); len(players) != 0 {
		t.Error("no players for national teams")
	}
}

func TestRoundLabels(t *testing.T) {
	for _, tc := range []struct{ slug, group, want string }{
		{"group-stage", "Group F", "Group F"},
		{"round-of-16", "", "Round of 16"},
		{"3rd-place-match", "", "Third place"},
		{"2024-international-friendly", "", ""},
		{"2026-world-cup-qualifying---conmebol", "", ""},
		{"relegation-playoffs", "", "Relegation Playoffs"},
		{"second-round", "2026 FIFA World Cup¤ AFC Qualifying - Group D", "Group D"},
	} {
		if got := roundLabel(tc.slug, tc.group); got != tc.want {
			t.Errorf("roundLabel(%q, %q) = %q, want %q", tc.slug, tc.group, got, tc.want)
		}
	}
}
