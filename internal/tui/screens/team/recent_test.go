package team_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func ip(n int) *int         { return &n }
func fp(f float64) *float64 { return &f }

// seedGames gives Arsenal six finished games across the league, a cup and
// Europe, and a fixture still to come, against fictional opponents.
func seedGames(t *testing.T, repos storage.Repositories) {
	t.Helper()
	ctx := context.Background()
	cup := models.Competition{ID: "eng-fa-cup", Name: "FA Cup", ShortName: "FAC", Country: "England", Gender: "men",
		Format: "cup", Category: models.CategoryCup}
	ucl := storagetest.ChampionsLeague()
	for _, c := range []models.Competition{cup, ucl} {
		if err := repos.UpsertCompetition(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, se := range []models.Season{
		{ID: "eng-fa-cup:2025", CompetitionID: "eng-fa-cup", Label: "2025"},
		{ID: ucl.ID + ":2025", CompetitionID: ucl.ID, Label: "2025"},
	} {
		if err := repos.UpsertSeason(ctx, se); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Wessex Rovers", "Northbridge", "Fenmoor", "Calder", "Ashby", "Dunmore", "Kelso"} {
		if err := repos.UpsertTeam(ctx, models.Team{ID: "eng-" + strings.ToLower(name[:4]), Name: name, Country: "England", Gender: "men"}); err != nil {
			t.Fatal(err)
		}
	}
	day := func(d int) time.Time { return time.Date(2026, 3, d, 15, 0, 0, 0, time.UTC) }
	game := func(id, comp, season string, d int, home, away string, hs, as int) models.Match {
		return models.Match{ID: id, CompetitionID: comp, SeasonID: season, Kickoff: day(d), Status: models.MatchFullTime,
			HomeTeamID: home, AwayTeamID: away, HomeScore: ip(hs), AwayScore: ip(as), Source: "test", UpdatedAt: day(28)}
	}
	pl := "eng-premier-league"
	matches := []models.Match{
		game("g1", pl, pl+":2025", 1, arsenal.ID, "eng-dunm", 0, 1), // oldest: off the strip
		game("g2", pl, pl+":2025", 4, "eng-ashb", arsenal.ID, 2, 2),
		game("g3", ucl.ID, ucl.ID+":2025", 7, arsenal.ID, "eng-cald", 3, 0),
		game("g4", "eng-fa-cup", "eng-fa-cup:2025", 10, "eng-wess", arsenal.ID, 1, 1),
		game("g5", pl, pl+":2025", 14, arsenal.ID, "eng-fenm", 1, 2),
		game("g6", pl, pl+":2025", 18, "eng-nort", arsenal.ID, 0, 2),
	}
	matches[3].Status, matches[3].HomePens, matches[3].AwayPens = models.MatchPens, ip(2), ip(4)
	matches[5].HomeStats = &models.MatchTeamStats{XG: fp(0.81)}
	matches[5].AwayStats = &models.MatchTeamStats{XG: fp(2.14)}
	fixture := game("g7", pl, pl+":2025", 25, arsenal.ID, "eng-kels", 0, 0)
	fixture.Status, fixture.HomeScore, fixture.AwayScore = models.MatchScheduled, nil, nil
	if err := repos.UpsertMatches(ctx, append(matches, fixture)); err != nil {
		t.Fatal(err)
	}
}

func TestTheRecentStripShowsFormAndTheLastFiveGames(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedGames(t, repos)
	deps := newDeps(repos)
	// The stored leagues have no short names, as in a database cached
	// before they did; the catalog supplies them.
	deps.Catalog = []core.CatalogEntry{
		{Competition: models.Competition{ID: "eng-premier-league", ShortName: "PL"}},
		{Competition: models.Competition{ID: "uefa-champions-league", ShortName: "UCL"}},
	}
	s := settled(t, deps, "", "")
	out := s.View()

	if got := s.(*team.Model).RecentGames(); len(got) != 5 || got[0].ID != "g6" {
		t.Fatalf("recent games %v", got)
	}
	// Oldest first, so it reads left to right in time: g2 D, g3 W, g4 D on
	// penalties, g5 L, g6 W.
	if !strings.Contains(out, "Recent  D W D L W") {
		t.Errorf("form missing\n---\n%s", out)
	}
	for _, want := range []string{"Northbridge", "2–0", "xG 2.1–0.8", "Wessex Rovers", "1–1 (4–2 p)", "FAC", "UCL", " PL "} {
		if !strings.Contains(out, want) {
			t.Errorf("strip lacks %q\n---\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Dunmore", "Kelso"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("the strip shows %s: only the last five finished games belong\n---\n%s", unwanted, out)
		}
	}
	// The club's tables are still there beneath it.
	if !strings.Contains(out, "Premier League") {
		t.Errorf("the seasons are gone\n---\n%s", out)
	}
}

// initMsgs runs a screen's Init batch and returns the messages it yields.
func initMsgs(t *testing.T, s core.Screen) []tea.Msg {
	t.Helper()
	var out []tea.Msg
	queue := []tea.Cmd{s.Init()}
	for len(queue) > 0 {
		cmd := queue[0]
		queue = queue[1:]
		if cmd == nil {
			continue
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		out = append(out, msg)
	}
	return out
}

func TestOpeningAClubQueuesItsRecentGamesOnlyWhenDue(t *testing.T) {
	repos := seedTwoCompetitions(t)
	for _, tc := range []struct {
		due  bool
		want int
	}{{true, 1}, {false, 0}} {
		deps := newDeps(repos)
		asked := 0
		deps.ClubDue = func(club models.Team) (string, bool) {
			asked++
			return "FAC 2026 not fetched yet", tc.due
		}
		var reqs []core.RefreshRequest
		for _, msg := range initMsgs(t, team.New(deps, arsenal, "", "")) {
			if r, ok := msg.(core.RefreshRequestMsg); ok {
				reqs = append(reqs, r.Requests...)
			}
		}
		if asked != 1 || len(reqs) != tc.want {
			t.Fatalf("due=%v: asked %d times, queued %+v", tc.due, asked, reqs)
		}
		if tc.due && (reqs[0].TeamID != arsenal.ID || !reqs[0].Automatic || reqs[0].Why == "") {
			t.Errorf("request %+v", reqs[0])
		}
	}
}

func TestTheStripSaysWhenItsGamesAreBeingFetched(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "", "")
	req := core.RefreshRequest{TeamID: arsenal.ID, Name: "Arsenal"}
	s, _ = s.Update(core.RefreshQueueMsg{Active: &req})
	if out := s.View(); !strings.Contains(out, "fetching recent games") {
		t.Errorf("no fetching note\n---\n%s", out)
	}
	s, _ = s.Update(core.RefreshQueueMsg{})
	if out := s.View(); !strings.Contains(out, "No recent games cached") {
		t.Errorf("no empty note\n---\n%s", out)
	}
}

// A cup opponent has games but no table: the strip is the screen, and its
// refresh is its recent games.
func TestAClubKnownOnlyFromItsGamesShowsThem(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedGames(t, repos)
	wessex, _ := repos.GetTeam(context.Background(), "eng-wess")
	s := tuitest.Resize(t, tuitest.Settle(t, team.New(newDeps(repos), wessex, "", "")), 110, 44)
	out := s.View()
	if !strings.Contains(out, "Recent") || !strings.Contains(out, "Arsenal") || strings.Contains(out, "No seasons") {
		t.Errorf("view\n---\n%s", out)
	}
	req, ok := s.(core.RefreshRequester).RefreshRequest()
	if !ok || req.TeamID != "eng-wess" {
		t.Errorf("refresh %+v, %v", req, ok)
	}
}

func TestMOpensTheGamesScreen(t *testing.T) {
	repos := seedTwoCompetitions(t)
	deps := newDeps(repos)
	var opened models.Team
	deps.Nav.Games = func(_ core.Deps, t models.Team) core.Screen { opened = t; return stubScreen{} }
	s := settled(t, deps, "", "")
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok || opened.ID != arsenal.ID {
		t.Errorf("m opened %+v", opened)
	}
}
