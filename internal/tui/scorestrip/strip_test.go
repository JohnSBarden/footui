package scorestrip

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// now is a Saturday afternoon.
var now = time.Date(2026, 10, 17, 15, 30, 0, 0, time.Local)

func ptr(n int) *int { return &n }

// game is a match id between home and away kicking off at now+at; a
// score makes it a result.
func game(id string, at time.Duration, home, away string, score ...int) models.Match {
	m := models.Match{ID: id, CompetitionID: "eng-premier-league", Kickoff: now.Add(at),
		HomeTeamID: home, AwayTeamID: away, Status: models.MatchScheduled}
	if len(score) == 2 {
		m.Status, m.HomeScore, m.AwayScore = models.MatchFullTime, ptr(score[0]), ptr(score[1])
	}
	return m
}

func ids(ms []models.Match) string {
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return strings.Join(out, " ")
}

const day = 24 * time.Hour

// A season's strip is the round just played and the one coming, not the
// rounds either side of them.
func TestRoundsAreTheLatestAndTheNext(t *testing.T) {
	ms := []models.Match{
		game("old", -8*day, "a", "b", 1, 0),
		game("fri", -day-20*time.Hour, "c", "d", 2, 2),
		game("sat", -2*time.Hour, "e", "f", 0, 1),
		game("live", -30*time.Minute, "g", "h"),
		game("next", 6*day, "a", "c"),
		game("next2", 7*day, "b", "d"),
		game("later", 13*day, "e", "g"),
	}
	if got := ids(Rounds(ms, now)); got != "fri sat live next next2" {
		t.Errorf("rounds = %q", got)
	}
	// A season that's over keeps its last round.
	if got := ids(Rounds(ms[:3], now.Add(60*day))); got != "fri sat" {
		t.Errorf("ended season = %q", got)
	}
}

// A team's strip is its last two games and its next.
func TestTeamWindow(t *testing.T) {
	ms := []models.Match{ // newest first, the way they're stored
		game("n2", 10*day, "a", "x"),
		game("n1", 3*day, "x", "b"),
		game("p1", -4*day, "x", "c", 1, 1),
		game("p2", -11*day, "d", "x", 0, 2),
		game("p3", -18*day, "x", "e", 3, 0),
	}
	if got := ids(TeamWindow(ms, now)); got != "p2 p1 n1" {
		t.Errorf("window = %q", got)
	}
	if got := ids(Ordered(append(ms, ms[0]))); got != "p3 p2 p1 n1 n2" {
		t.Errorf("ordered = %q, want no repeats", got)
	}
}

func strip(ms ...models.Match) Strip {
	teams := map[string]models.Team{}
	for _, m := range ms {
		for _, id := range []string{m.HomeTeamID, m.AwayTeamID} {
			teams[id] = models.Team{ID: id, Name: strings.ToUpper(id) + " FC", ShortName: strings.ToUpper(id)}
		}
	}
	return Strip{Matches: Ordered(ms), Teams: teams, Now: now, ShowComp: true,
		Comps: map[string]models.Competition{"eng-premier-league": {ID: "eng-premier-league", ShortName: "PL"}}}
}

// The strip opens with what's on now and next at its right, results
// filling the rest, and scrolls a page at a time.
func TestStripOpensOnNowAndScrolls(t *testing.T) {
	var ms []models.Match
	for i := range 10 {
		ms = append(ms, game(string(rune('a'+i)), time.Duration(i-10)*day, "h", "v", 1, 0))
	}
	ms = append(ms, game("next", day, "h", "v"), game("after", 2*day, "h", "v"))
	s := strip(ms...)
	// 120 columns hold five tiles.
	if n, _ := fit(120); n != 5 {
		t.Fatalf("tiles at 120 = %d", n)
	}
	if got := s.start(5); s.Matches[got].ID != "h" {
		t.Errorf("opens on %q, want three results then the two fixtures", s.Matches[got].ID)
	}
	s.Scroll(-1, 120)
	if got := s.start(5); got != 2 {
		t.Errorf("a page left starts at %d", got)
	}
	s.Scroll(-1, 120)
	s.Scroll(-1, 120)
	if got := s.start(5); got != 0 {
		t.Errorf("scrolling stops at the first game, at %d", got)
	}
	s.Scroll(1, 120)
	s.Scroll(1, 120)
	s.Scroll(1, 120)
	if got := s.start(5); got != len(s.Matches)-5 {
		t.Errorf("scrolling stops at the last page, at %d", got)
	}
}

func TestStripView(t *testing.T) {
	live := game("live", -40*time.Minute, "ars", "che")
	live.Status, live.HomeScore, live.AwayScore = models.MatchInPlay, ptr(1), ptr(0)
	pens := game("pens", -day, "liv", "eve", 1, 1)
	pens.Status, pens.HomePens, pens.AwayPens = models.MatchPens, ptr(3), ptr(4)
	s := strip(
		game("ft", -2*day, "mci", "tot", 2, 1),
		pens,
		live,
		game("due", -5*time.Hour, "new", "ful"),
		game("ko", day+4*time.Hour, "bha", "bou"),
	)
	out := ansi.Strip(s.View(styles.Plain(), 120))
	lines := strings.Split(out, "\n")
	if len(lines) != Lines {
		t.Fatalf("%d lines:\n%s", len(lines), out)
	}
	for _, want := range []string{
		"Thu 15 Oct", "Yesterday", "Today", "Tomorrow",
		"PL", "FT", "PENS 3–4", "● LIVE", "awaiting", "19:30",
		"MCI", "TOT", "ARS", "CHE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	// "Today" heads the first of today's games only.
	if strings.Count(lines[0], "Today") != 1 {
		t.Errorf("day line %q", lines[0])
	}
	if s.Empty() || (Strip{}).Height() != 0 || (Strip{}).View(styles.Plain(), 80) != "" {
		t.Error("an empty strip draws nothing")
	}
}

func TestShorten(t *testing.T) {
	for name, want := range map[string]string{
		"Manchester United":        "M. United",
		"Manchester City":          "M. City",
		"D.C. United":              "D.C. United",
		"Wolverhampton Wanderers":  "W. Wanderers",
		"Borussia Mönchengladbach": "B. Möncheng…",
	} {
		if got := shorten(name, 12); got != want {
			t.Errorf("shorten(%q) = %q, want %q", name, got, want)
		}
	}
}

// More games than fit get arrows on the side they're off.
func TestStripArrows(t *testing.T) {
	var ms []models.Match
	for i := range 8 {
		ms = append(ms, game(string(rune('a'+i)), time.Duration(i-4)*day, "h", "v", 1, 0))
	}
	s := strip(ms...)
	out := ansi.Strip(s.View(styles.Plain(), 60))
	if !strings.Contains(out, "‹") || strings.Contains(out, "›") {
		t.Errorf("opened at the latest, so only the left arrow:\n%s", out)
	}
	s.Scroll(-1, 60)
	s.Scroll(-1, 60)
	s.Scroll(-1, 60)
	out = ansi.Strip(s.View(styles.Plain(), 60))
	if strings.Contains(out, "‹") || !strings.Contains(out, "›") {
		t.Errorf("at the start, only the right arrow:\n%s", out)
	}
}
