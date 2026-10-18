package espn

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// athleteServer serves the core API's per-athlete statistics and one
// club roster from testdata.
func athleteServer(t *testing.T) *Provider {
	t.Helper()
	routes := map[string]string{
		"/core/eng.1/seasons/2025/types/1/athletes/196176/statistics": "athlete_eng.1_2025_keeper.json",
		"/core/eng.1/seasons/2025/types/1/athletes/236322/statistics": "athlete_eng.1_2025_defender.json",
		"/core/eng.1/seasons/2023/types/1/athletes/236322/statistics": "athlete_eng.1_2023_defender.json",
		"/site/site/v2/sports/soccer/usa.1/teams/182/roster":          "roster_usa.1_182_2025.json",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	return newWithClient(NewClient().withBaseURLs(srv.URL+"/site", srv.URL+"/core"))
}

func deref(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}

// The Premier League isn't one of ESPN's synced competitions (understat
// serves it), but its players' detail still comes from ESPN.
func TestAthleteSeasonKeeper(t *testing.T) {
	d, err := athleteServer(t).AthleteSeason(ctxBkgnd, "eng-premier-league", 2025, "196176", true)
	if err != nil {
		t.Fatal(err)
	}
	if deref(d.CleanSheets) != 19 || deref(d.GoalsConceded) != 26 || deref(d.Saves) != 60 || deref(d.Minutes) != 3330 {
		t.Errorf("keeper detail = cs %d ga %d saves %d min %d", deref(d.CleanSheets), deref(d.GoalsConceded), deref(d.Saves), deref(d.Minutes))
	}
}

func TestAthleteSeasonDefender(t *testing.T) {
	d, err := athleteServer(t).AthleteSeason(ctxBkgnd, "eng-premier-league", 2025, "236322", false)
	if err != nil {
		t.Fatal(err)
	}
	if deref(d.Tackles) != 38 || deref(d.Interceptions) != 23 || deref(d.Clearances) != 182 || deref(d.CleanSheets) != 17 {
		t.Errorf("defender detail = tkl %d int %d clr %d cs %d", deref(d.Tackles), deref(d.Interceptions), deref(d.Clearances), deref(d.CleanSheets))
	}
	if d.PassPct == nil || *d.PassPct < 0.881 || *d.PassPct > 0.883 {
		t.Errorf("pass pct = %v, want 0.882", d.PassPct)
	}
	if d.Saves != nil {
		t.Errorf("an outfield player's saves must be dropped, got %d", *d.Saves)
	}
}

// ESPN's older seasons carry zeros for the defensive stats it didn't
// record, and the club's saves against every player. Neither is a stat.
func TestAthleteSeasonZeroDefenceIsNotRecorded(t *testing.T) {
	d, err := athleteServer(t).AthleteSeason(ctxBkgnd, "eng-premier-league", 2023, "236322", false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Tackles != nil || d.Interceptions != nil || d.Clearances != nil {
		t.Errorf("zero defence must be nil: tkl %v int %v clr %v", d.Tackles, d.Interceptions, d.Clearances)
	}
	if d.PassPct != nil {
		t.Errorf("a zero pass rate must be nil, got %v", *d.PassPct)
	}
	if d.Saves != nil {
		t.Errorf("saves for a defender must be nil, got %d", *d.Saves)
	}
	if deref(d.CleanSheets) != 17 || deref(d.Minutes) != 3046 {
		t.Errorf("clean sheets and minutes are kept: cs %d min %d", deref(d.CleanSheets), deref(d.Minutes))
	}
}

func TestSquadPositions(t *testing.T) {
	squad, err := athleteServer(t).Squad(ctxBkgnd, "usa-mls", 2025, "182")
	if err != nil {
		t.Fatal(err)
	}
	if len(squad) != 4 {
		t.Fatalf("got %d members, want 4", len(squad))
	}
	if squad[2].ID != "900003" || squad[2].Name != "Sam Okafor" || squad[2].Position != "G" {
		t.Errorf("member = %+v", squad[2])
	}
}
