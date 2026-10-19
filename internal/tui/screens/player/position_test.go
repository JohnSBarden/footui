package player_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/player"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

const (
	pl     = "eng-premier-league"
	pl2025 = "eng-premier-league:2025"
	pl2024 = "eng-premier-league:2024"
)

// seedArsenalSeason caches the 2025-26 Premier League with Arsenal's xGA
// (33.1 in 38) and, when given, the line and detail for one player.
func seedArsenalSeason(t *testing.T, repos storage.Repositories, season, year string) {
	t.Helper()
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), year, []storagetest.TeamRow{
		{ID: "eng-arsenal", Name: "Arsenal", Position: 2, MP: 38, Pts: 85, GF: 71, GA: 27, XG: 70.2, XGA: 33.1},
	})
}

func seedLine(t *testing.T, repos storage.Repositories, p models.Player, line models.PlayerSeasonStats, d *models.PlayerSeasonDetail) {
	t.Helper()
	ctx := context.Background()
	must(t, repos.UpsertPlayer(ctx, p))
	line.PlayerID, line.TeamID, line.CompetitionID, line.Source = p.ID, "eng-arsenal", pl, "understat"
	if line.SeasonID == "" {
		line.SeasonID = pl2025
	}
	must(t, repos.UpsertPlayerSeasonStats(ctx, line))
	if d != nil {
		d.PlayerID, d.TeamID, d.CompetitionID, d.SeasonID = p.ID, "eng-arsenal", pl, line.SeasonID
		d.FetchedAt = time.Now()
		must(t, repos.UpsertPlayerSeasonDetail(ctx, *d))
	}
}

var (
	raya    = models.Player{ID: "understat-9676", Name: "David Raya", Position: "GK", Gender: "men"}
	gabriel = models.Player{ID: "understat-5613", Name: "Gabriel", Position: "D", Gender: "men"}
	rice    = models.Player{ID: "understat-5743", Name: "Declan Rice", Position: "M S", Gender: "men"}
	saka    = models.Player{ID: "understat-7322", Name: "Bukayo Saka", Position: "F M S", Gender: "men"}
)

func rayaRepos(t *testing.T, withDetail bool) storage.Repositories {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	var d *models.PlayerSeasonDetail
	if withDetail {
		d = &models.PlayerSeasonDetail{Position: "G", Appearances: ip(37), Minutes: ip(3330),
			CleanSheets: ip(19), GoalsConceded: ip(26), Saves: ip(60)}
	}
	seedLine(t, repos, raya, models.PlayerSeasonStats{Appearances: 37, Minutes: 3330, XG: fp(0), XA: fp(0.1)}, d)
	return repos
}

func gabrielRepos(t *testing.T) storage.Repositories {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	seedLine(t, repos, gabriel, models.PlayerSeasonStats{Appearances: 32, Minutes: 2748, Goals: 3, Assists: 4,
		XG: fp(4.6), XA: fp(2.7), Shots: ip(27), KeyPasses: ip(7)},
		&models.PlayerSeasonDetail{Position: "D", Appearances: ip(32), Minutes: ip(2751), CleanSheets: ip(17),
			GoalsConceded: ip(20), Tackles: ip(38), Interceptions: ip(23), Clearances: ip(182), PassPct: fp(0.882)})
	return repos
}

func view(t *testing.T, repos storage.Repositories, p models.Player) string {
	t.Helper()
	s := tuitest.Settle(t, player.New(newDeps(repos), p, "eng-arsenal"))
	return tuitest.Resize(t, s, 120, 40).View()
}

func wantAll(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("view missing %q\n---\n%s", w, out)
		}
	}
}

func TestKeeperTiles(t *testing.T) {
	out := view(t, rayaRepos(t, true), raya)
	wantAll(t, out, "CLEAN SHEETS", "in 37 apps · 51%", "SAVES", "70% of shots on target",
		"CONCEDED", "0.70 per 90", "VS EXPECTED", "+6.2")
	if strings.Contains(out, "KEY PASSES") {
		t.Errorf("a keeper has no key passes tile\n%s", out)
	}
}

func TestDefenderTiles(t *testing.T) {
	out := view(t, gabrielRepos(t), gabriel)
	wantAll(t, out, "CLEAN SHEETS", "TACKLES", "1.24", "INTERCEPTIONS", "0.75", "CLEARANCES", "5.95", "PASSING", "88%")
}

func TestMidfielderTiles(t *testing.T) {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	seedLine(t, repos, rice, models.PlayerSeasonStats{Appearances: 36, Minutes: 3111, Goals: 4, Assists: 5,
		XG: fp(3.7), XA: fp(8.2), Shots: ip(40), KeyPasses: ip(63)},
		&models.PlayerSeasonDetail{Position: "M", Minutes: ip(3099), Tackles: ip(70), Interceptions: ip(37), PassPct: fp(0.873)})
	out := view(t, repos, rice)
	wantAll(t, out, "GOALS", "from 3.7 xG", "ASSISTS", "from 8.2 xA", "KEY PASSES", "1.82", "TACKLES + INT", "3.11", "87%")
}

func TestForwardTiles(t *testing.T) {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	seedLine(t, repos, saka, models.PlayerSeasonStats{Appearances: 31, Minutes: 2239, Goals: 7, Assists: 5,
		XG: fp(8.7), XA: fp(8.6), Shots: ip(71), KeyPasses: ip(62)},
		&models.PlayerSeasonDetail{Position: "F", Minutes: ip(2225), ShotsOnTarget: ip(27)})
	out := view(t, repos, saka)
	wantAll(t, out, "GOALS", "from 8.7 xG", "SHOTS", "2.85", "ON TARGET", "38%", "27 of 71", "MINUTES")
}

func TestStandoutCallout(t *testing.T) {
	repos := gabrielRepos(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		p := models.Player{ID: fmt.Sprintf("d%d", i), Name: fmt.Sprintf("Defender %d", i), Position: "D"}
		must(t, repos.UpsertPlayer(ctx, p))
		must(t, repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{PlayerID: p.ID, TeamID: "eng-other",
			CompetitionID: pl, SeasonID: pl2025, Minutes: 2000, Goals: 3, Assists: 3, XG: fp(0.5), XA: fp(2),
			Shots: ip(20), KeyPasses: ip(20), Source: "understat"}))
	}
	out := view(t, repos, gabriel)
	wantAll(t, out, "▲ unusual for a defender", "4.6 xG", "0.15 per 90", "top 1% of Premier League defenders")
}

func TestUnrecordedSeasonShowsDash(t *testing.T) {
	repos := gabrielRepos(t)
	seedArsenalSeason(t, repos, pl2024, "2024")
	seedLine(t, repos, gabriel, models.PlayerSeasonStats{SeasonID: pl2024, Appearances: 28, Minutes: 2376, Goals: 3, XG: fp(4.3)},
		&models.PlayerSeasonDetail{Position: "D", Minutes: ip(2365), CleanSheets: ip(10), GoalsConceded: ip(22), PassPct: fp(0.908)})
	out := view(t, repos, gabriel)
	wantAll(t, out, "no tackles, interceptions or clearances recorded that season")
	var row string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "2024-25") && strings.Contains(l, "Arsenal") {
			row = l
		}
	}
	if !strings.Contains(row, "—") || !strings.Contains(row, "91%") {
		t.Errorf("2024-25 row should show — for tackles and keep its passing: %q", row)
	}
}

func TestPlayerWithoutDetailFallsBack(t *testing.T) {
	out := view(t, rayaRepos(t, false), raya)
	wantAll(t, out, "MINUTES", "David Raya")
	if strings.Contains(out, "CLEAN SHEETS") {
		t.Errorf("no detail, no clean sheets tile\n%s", out)
	}
}

func TestKeeperChartIsCleanSheets(t *testing.T) {
	out := view(t, rayaRepos(t, true), raya)
	wantAll(t, out, "CLEAN SHEETS", "19 in 37")
}

func TestKeeperTableAndFootnote(t *testing.T) {
	out := view(t, rayaRepos(t, true), raya)
	wantAll(t, out, "CS", "GA", "Saves", "Sv%", "xGA*", "32.2", "* the club's xGA, scaled by their share of minutes")
}

func TestRefreshingLine(t *testing.T) {
	s := tuitest.Settle(t, player.New(newDeps(rayaRepos(t, true)), raya, "eng-arsenal"))
	s = tuitest.Resize(t, s, 120, 40)
	s, _ = s.Update(core.RefreshQueueMsg{Active: &core.RefreshRequest{PlayerID: raya.ID}})
	if !strings.Contains(s.View(), "🔄 refreshing") {
		t.Errorf("no refreshing line\n%s", s.View())
	}
}

// A season barely recorded next to the player's best reads as —, not as
// a midfielder who stopped defending.
func TestSparseSeasonShowsDash(t *testing.T) {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	seedArsenalSeason(t, repos, pl2024, "2024")
	seedLine(t, repos, rice, models.PlayerSeasonStats{Appearances: 36, Minutes: 3111, Goals: 4, Assists: 5},
		&models.PlayerSeasonDetail{Position: "M", Minutes: ip(3099), Tackles: ip(70), Interceptions: ip(37), Clearances: ip(78), PassPct: fp(0.873)})
	seedLine(t, repos, rice, models.PlayerSeasonStats{SeasonID: pl2024, Appearances: 35, Minutes: 2848, Goals: 4, Assists: 7},
		&models.PlayerSeasonDetail{Position: "M", Minutes: ip(2876), Tackles: ip(7), Interceptions: ip(4), Clearances: ip(7), PassPct: fp(0.91)})
	out := view(t, repos, rice)
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "2024-25") && strings.Contains(l, "Arsenal") && strings.Contains(l, "0.34") {
			t.Errorf("the sparse season should show —: %q", l)
		}
	}
	wantAll(t, out, "no tackles, interceptions or clearances recorded that season")
}

// For an ESPN league the line has no minutes: rates and the minutes tile
// come from the detail.
func TestForwardRatesUseDetailMinutes(t *testing.T) {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	seedLine(t, repos, saka, models.PlayerSeasonStats{Appearances: 28, Goals: 25, Assists: 14, Shots: ip(110)},
		&models.PlayerSeasonDetail{Position: "F", Minutes: ip(2400), ShotsOnTarget: ip(45)})
	out := view(t, repos, saka)
	wantAll(t, out, "4.12", "per 90 · 110", "2400")
}

// ESPN's synced rosters carry no minutes; the player's detail does.
func TestTableUsesDetailMinutes(t *testing.T) {
	repos := storagetest.New(t)
	seedArsenalSeason(t, repos, pl2025, "2025")
	seedLine(t, repos, raya, models.PlayerSeasonStats{Appearances: 2},
		&models.PlayerSeasonDetail{Position: "G", Appearances: ip(2), Minutes: ip(180), CleanSheets: ip(2), GoalsConceded: ip(0), Saves: ip(3)})
	out := view(t, repos, raya)
	var row string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "2025-26") && strings.Contains(l, "Arsenal") {
			row = l
		}
	}
	if !strings.Contains(row, " 180 ") {
		t.Errorf("Mins should come from the detail: %q", row)
	}
}

// Opening a player asks the root app to bring their detail up to date.
func TestOpenRequestsPlayerRefresh(t *testing.T) {
	m := player.New(newDeps(rayaRepos(t, true)), raya, "eng-arsenal")
	var found *core.RefreshRequest
	var walk func(cmd tea.Cmd)
	walk = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				walk(c)
			}
		case core.RefreshRequestMsg:
			for _, r := range msg.Requests {
				if r.PlayerID != "" {
					found = &r
				}
			}
		}
	}
	walk(m.Init())
	if found == nil || found.PlayerID != raya.ID || found.Name != "David Raya" || !found.Automatic {
		t.Errorf("request = %+v", found)
	}
}
