package playerstats

import (
	"fmt"
	"math"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
)

func f(v float64) *float64 { return &v }
func n(v int) *int         { return &v }

func TestGroupFromESPN(t *testing.T) {
	for code, want := range map[string]Group{"G": GK, "D": DEF, "M": MID, "F": FWD} {
		if got := GroupOf(code, "F M S", nil); got != want {
			t.Errorf("ESPN %q → %v, want %v", code, got, want)
		}
	}
}

func TestGroupFromUnderstatCodes(t *testing.T) {
	scorer := []models.PlayerSeasonStats{{Goals: 10, Assists: 3}}
	creator := []models.PlayerSeasonStats{{Goals: 2, Assists: 9}}
	cases := []struct {
		code  string
		lines []models.PlayerSeasonStats
		want  Group
	}{
		{"GK S", nil, GK},
		{"D", scorer, DEF},
		{"D M S", creator, MID},
		{"D M S", scorer, MID}, // a scoring D/M is still not a forward
		{"F M S", scorer, FWD},
		{"F M S", creator, MID},
		{"D F", creator, DEF},
		{"", scorer, FWD},
		{"", creator, MID},
	}
	for _, c := range cases {
		if got := GroupOf("", c.code, c.lines); got != c.want {
			t.Errorf("%q with %+v → %v, want %v", c.code, c.lines, got, c.want)
		}
	}
}

func TestGroupNames(t *testing.T) {
	if DEF.Short() != "DEF" || DEF.Plural() != "defenders" || GK.Singular() != "keeper" {
		t.Errorf("names: %q %q %q", DEF.Short(), DEF.Plural(), GK.Singular())
	}
}

// baseline147 is 147 defenders with 1000 minutes each; the first `better`
// have more xG than the player's 4.6 in 2748 minutes.
func baseline147(better int) ([]models.PlayerSeasonStats, map[string]Group) {
	var lines []models.PlayerSeasonStats
	groups := map[string]Group{}
	for i := 0; i < 146; i++ {
		id := fmt.Sprintf("d%d", i)
		xg := 0.05
		if i < better {
			xg = 2.0 // 0.18 per 90, above his 0.15
		}
		lines = append(lines, models.PlayerSeasonStats{PlayerID: id, Minutes: 1000, Goals: 2, Assists: 2, XG: f(xg)})
		groups[id] = DEF
	}
	return lines, groups
}

var gabriel = models.PlayerSeasonStats{PlayerID: "gabriel", Minutes: 2748, Goals: 3, Assists: 4, XG: f(4.6), XA: f(2.7)}

func TestStandoutTopPercent(t *testing.T) {
	lines, groups := baseline147(13)
	lines = append(lines, gabriel)
	groups["gabriel"] = DEF
	got := Standouts(gabriel, DEF, lines, groups)
	if len(got) == 0 || got[0].Stat != "xG" || got[0].TopPct != 9 || got[0].Value != 4.6 {
		t.Fatalf("standouts = %+v, want xG top 9%%", got)
	}
	if math.Abs(got[0].Per90-0.1507) > 0.001 {
		t.Errorf("per 90 = %.4f", got[0].Per90)
	}
}

func TestStandoutExcludesOwnStats(t *testing.T) {
	var lines []models.PlayerSeasonStats
	groups := map[string]Group{}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("f%d", i)
		lines = append(lines, models.PlayerSeasonStats{PlayerID: id, Minutes: 1000, Goals: 1, Shots: n(5)})
		groups[id] = FWD
	}
	star := models.PlayerSeasonStats{PlayerID: "star", Minutes: 2000, Goals: 30, Shots: n(120)}
	lines = append(lines, star)
	groups["star"] = FWD
	for _, s := range Standouts(star, FWD, lines, groups) {
		if s.Stat == "goals" || s.Stat == "shots" {
			t.Errorf("a forward's %s never stands out", s.Stat)
		}
	}
}

func TestStandoutNeedsMinutes(t *testing.T) {
	lines, groups := baseline147(0)
	short := gabriel
	short.Minutes = 800
	if got := Standouts(short, DEF, append(lines, short), groups); len(got) != 0 {
		t.Errorf("under 900 minutes: %+v", got)
	}
}

// A keeper with no assists among keepers with none isn't "top 0%".
func TestStandoutNeedsNonZero(t *testing.T) {
	var lines []models.PlayerSeasonStats
	groups := map[string]Group{}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("k%d", i)
		lines = append(lines, models.PlayerSeasonStats{PlayerID: id, Minutes: 3000})
		groups[id] = GK
	}
	if got := Standouts(lines[0], GK, lines, groups); len(got) != 0 {
		t.Errorf("zeros stood out: %+v", got)
	}
}

func TestStandoutNeedsABaseline(t *testing.T) {
	lines := []models.PlayerSeasonStats{gabriel, {PlayerID: "x", Minutes: 1000, XG: f(0.1)}}
	groups := map[string]Group{"gabriel": DEF, "x": DEF}
	if got := Standouts(gabriel, DEF, lines, groups); len(got) != 0 {
		t.Errorf("two players are no baseline: %+v", got)
	}
}

func TestStandoutAtMostTwo(t *testing.T) {
	var lines []models.PlayerSeasonStats
	groups := map[string]Group{}
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("d%d", i)
		lines = append(lines, models.PlayerSeasonStats{PlayerID: id, Minutes: 1000, Goals: 0, Assists: 0,
			XG: f(0.01), XA: f(0.01), Shots: n(1), KeyPasses: n(1)})
		groups[id] = DEF
	}
	star := models.PlayerSeasonStats{PlayerID: "star", Minutes: 3000, Goals: 9, Assists: 9,
		XG: f(9), XA: f(9), Shots: n(90), KeyPasses: n(90)}
	got := Standouts(star, DEF, append(lines, star), groups)
	if len(got) != 2 {
		t.Errorf("got %d standouts, want at most 2", len(got))
	}
}

// Rice's 2024-25 is 7 tackles, 4 interceptions, 7 clearances in 2876
// minutes against 185 in 3099 the next season: ESPN barely recorded it.
func TestClearSparseDefence(t *testing.T) {
	rice24 := &models.PlayerSeasonDetail{Minutes: n(2876), Tackles: n(7), Interceptions: n(4), Clearances: n(7), PassPct: f(0.91)}
	rice25 := &models.PlayerSeasonDetail{Minutes: n(3099), Tackles: n(70), Interceptions: n(37), Clearances: n(78)}
	cameo := &models.PlayerSeasonDetail{Minutes: n(300), Tackles: n(0), Interceptions: n(0), Clearances: n(1)}
	ClearSparseDefence([]*models.PlayerSeasonDetail{rice24, rice25, cameo})
	if rice24.Tackles != nil || rice24.Interceptions != nil || rice24.Clearances != nil {
		t.Errorf("a season at a tenth of his best is unrecorded: %+v", rice24)
	}
	if rice24.PassPct == nil {
		t.Error("passing is kept")
	}
	if rice25.Tackles == nil || cameo.Tackles == nil {
		t.Error("his best season, and one too short to judge, are kept")
	}

	// A striker who rarely defends, every season, is just that.
	a := &models.PlayerSeasonDetail{Minutes: n(3000), Tackles: n(10), Interceptions: n(3), Clearances: n(15)}
	b := &models.PlayerSeasonDetail{Minutes: n(2800), Tackles: n(8), Interceptions: n(2), Clearances: n(12)}
	ClearSparseDefence([]*models.PlayerSeasonDetail{a, b})
	if a.Tackles == nil || b.Tackles == nil {
		t.Error("a consistently quiet player keeps his numbers")
	}
}

func TestKeeperExpected(t *testing.T) {
	team := models.TeamSeasonStats{MatchesPlayed: 38, XGA: f(33.1)}
	xga, ok := KeeperExpected(3330, team)
	if !ok || math.Abs(xga-32.25) > 0.05 {
		t.Errorf("xga = %.2f, %v; want 32.2", xga, ok)
	}
}

func TestKeeperExpectedNeedsXGA(t *testing.T) {
	if _, ok := KeeperExpected(3330, models.TeamSeasonStats{MatchesPlayed: 30}); ok {
		t.Error("no xGA, no estimate")
	}
	if _, ok := KeeperExpected(3330, models.TeamSeasonStats{XGA: f(30)}); ok {
		t.Error("no matches, no estimate")
	}
}
