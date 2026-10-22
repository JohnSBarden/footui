package dashboard_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }

const plSeason = "eng-premier-league:2025"

// starPlayer caches and stars a player's Arsenal line; d may be nil.
func starPlayer(t *testing.T, p models.Player, line models.PlayerSeasonStats, d *models.PlayerSeasonDetail) (storage.Repositories, core.Deps) {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: "eng-arsenal", Name: "Arsenal", Position: 2, MP: 38, Pts: 85, GF: 71, GA: 27, XG: 70.2, XGA: 33.1},
	})
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(repos.UpsertPlayer(ctx, p))
	line.PlayerID, line.TeamID, line.CompetitionID, line.SeasonID, line.Source = p.ID, "eng-arsenal", "eng-premier-league", plSeason, "understat"
	check(repos.UpsertPlayerSeasonStats(ctx, line))
	if d != nil {
		d.PlayerID, d.TeamID, d.CompetitionID, d.SeasonID, d.FetchedAt = p.ID, "eng-arsenal", "eng-premier-league", plSeason, storagetest.Fixed
		check(repos.UpsertPlayerSeasonDetail(ctx, *d))
	}
	fav := models.Favorite{Kind: models.FavoritePlayer, ID: p.ID, AddedAt: storagetest.Fixed}
	check(repos.AddFavorite(ctx, fav))
	return repos, core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites([]models.Favorite{fav})}
}

func cardView(t *testing.T, deps core.Deps) string {
	t.Helper()
	return ansi.Strip(load(t, deps, 120).View())
}

func wantIn(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("card missing %q\n%s", w, out)
		}
	}
}

func TestKeeperCard(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-raya", Name: "David Raya", Position: "GK"},
		models.PlayerSeasonStats{Appearances: 37, Minutes: 3330},
		&models.PlayerSeasonDetail{Position: "G", Appearances: ip(37), Minutes: ip(3330), CleanSheets: ip(19), GoalsConceded: ip(26), Saves: ip(60)})
	wantIn(t, cardView(t, deps), "GK", "19 clean sheets · 26 conceded", "60 saves · 70% saved", "+6.2 vs expected (est.)")
}

func TestDefenderCard(t *testing.T) {
	repos, deps := starPlayer(t, models.Player{ID: "p-gabriel", Name: "Gabriel", Position: "D"},
		models.PlayerSeasonStats{Appearances: 32, Minutes: 2748, Goals: 3, Assists: 4, XG: fp(4.6), XA: fp(2.7), Shots: ip(27), KeyPasses: ip(7)},
		&models.PlayerSeasonDetail{Position: "D", Appearances: ip(32), Minutes: ip(2751), CleanSheets: ip(17), Tackles: ip(38), Interceptions: ip(23), Clearances: ip(182), PassPct: fp(0.882)})
	for i := 0; i < 20; i++ {
		p := models.Player{ID: fmt.Sprintf("d%d", i), Name: fmt.Sprintf("Defender %d", i), Position: "D"}
		if err := repos.UpsertPlayer(ctx, p); err != nil {
			t.Fatal(err)
		}
		if err := repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{PlayerID: p.ID, TeamID: "eng-other",
			CompetitionID: "eng-premier-league", SeasonID: plSeason, Minutes: 2000, Goals: 3, Assists: 3,
			XG: fp(0.5), XA: fp(2), Shots: ip(20), KeyPasses: ip(20), Source: "understat"}); err != nil {
			t.Fatal(err)
		}
	}
	wantIn(t, cardView(t, deps), "DEF", "17 clean sheets · 32 apps", "1.24 tkl · 0.75 int per 90", "▲ 4.6 xG, top 1% of defenders")
}

func TestMidfielderCard(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-rice", Name: "Declan Rice", Position: "M S"},
		models.PlayerSeasonStats{Appearances: 36, Minutes: 3111, Goals: 4, Assists: 5, XG: fp(3.7), XA: fp(8.2), Shots: ip(40), KeyPasses: ip(63)},
		&models.PlayerSeasonDetail{Position: "M", Minutes: ip(3099), Tackles: ip(70), Interceptions: ip(37), PassPct: fp(0.873)})
	wantIn(t, cardView(t, deps), "MID", "4 goals · 5 assists", "xG 3.7 · xA 8.2 · 1.82 KP/90", "3.11 tkl+int/90 · 87% pass")
}

func TestForwardCard(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-saka2", Name: "Bukayo Saka", Position: "F M S"},
		models.PlayerSeasonStats{Appearances: 31, Minutes: 2239, Goals: 7, Assists: 5, XG: fp(8.7), XA: fp(8.6), Shots: ip(71), KeyPasses: ip(62)},
		&models.PlayerSeasonDetail{Position: "F", Minutes: ip(2225), ShotsOnTarget: ip(27)})
	wantIn(t, cardView(t, deps), "FWD", "7 goals · 5 assists", "from 8.7 xG · 2.85 shots/90", "38% on target")
}

// ESPN lines have no minutes or key passes; minutes come from the detail.
func TestESPNLeagueCardsUseDetailMinutes(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-messi", Name: "Lionel Messi", Position: "F"},
		models.PlayerSeasonStats{Appearances: 28, Goals: 25, Assists: 14, Shots: ip(110)},
		&models.PlayerSeasonDetail{Position: "F", Minutes: ip(2400), ShotsOnTarget: ip(45)})
	out := cardView(t, deps)
	wantIn(t, out, "4.12 shots/90", "41% on target · 2400 min")
	if strings.Contains(out, "— shots/90") || strings.Contains(out, " 0 min") {
		t.Errorf("rates must use the detail's minutes\n%s", out)
	}

	_, deps = starPlayer(t, models.Player{ID: "p-labonta", Name: "Lo'eau LaBonta", Position: "M"},
		models.PlayerSeasonStats{Appearances: 26, Goals: 1},
		&models.PlayerSeasonDetail{Position: "M", Minutes: ip(2200), Tackles: ip(50), Interceptions: ip(31), PassPct: fp(0.81)})
	if out := cardView(t, deps); strings.Contains(out, "KP/90") {
		t.Errorf("no key passes recorded, none shown\n%s", out)
	}
}

func TestCardSparseSeasonLeavesDefendingOff(t *testing.T) {
	repos, deps := starPlayer(t, models.Player{ID: "p-gabriel", Name: "Gabriel", Position: "D"},
		models.PlayerSeasonStats{Appearances: 28, Minutes: 2376},
		&models.PlayerSeasonDetail{Position: "D", Minutes: ip(2365), CleanSheets: ip(10), Tackles: ip(1), Interceptions: ip(1), Clearances: ip(7)})
	// An older, fully recorded season in the same competition.
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2024", nil)
	if err := repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{PlayerID: "p-gabriel", TeamID: "eng-arsenal",
		CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2024", Minutes: 2700, Source: "understat"}); err != nil {
		t.Fatal(err)
	}
	if err := repos.UpsertPlayerSeasonDetail(ctx, models.PlayerSeasonDetail{PlayerID: "p-gabriel", TeamID: "eng-arsenal",
		CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2024", Minutes: ip(2700),
		Tackles: ip(40), Interceptions: ip(25), Clearances: ip(170), FetchedAt: storagetest.Fixed}); err != nil {
		t.Fatal(err)
	}
	if out := cardView(t, deps); strings.Contains(out, "tkl ·") {
		t.Errorf("a barely recorded season shows no tackle line\n%s", out)
	}
}

// ESPN's 2011 season has no minutes, so no rate can be worked out.
func TestForwardCardWithoutMinutesShowsTotals(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-henry", Name: "Thierry Henry", Position: "F"},
		models.PlayerSeasonStats{Appearances: 29, Goals: 15, Assists: 3, Shots: ip(70)},
		&models.PlayerSeasonDetail{Position: "F", Appearances: ip(29), Minutes: ip(0), CleanSheets: ip(0), GoalsConceded: ip(0),
			Tackles: ip(0), Interceptions: ip(0), Clearances: ip(3)})
	out := cardView(t, deps)
	wantIn(t, out, "70 shots · 29 apps")
	if strings.Contains(out, "— shots") {
		t.Errorf("no rate without minutes\n%s", out)
	}
}

// Without shots on target, a forward's second line is their playing time.
func TestForwardCardWithoutAccuracyShowsMinutes(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-zlatan", Name: "Zlatan Ibrahimovic", Position: "F"},
		models.PlayerSeasonStats{Appearances: 31, Goals: 31, Assists: 8, Shots: ip(159)},
		&models.PlayerSeasonDetail{Position: "F", Appearances: ip(31), Minutes: ip(2790)})
	wantIn(t, cardView(t, deps), "5.13 shots/90", "2790 min · 31 apps")
}

func TestCardWithoutDetail(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-raya", Name: "David Raya", Position: "GK"},
		models.PlayerSeasonStats{Appearances: 37, Minutes: 3330}, nil)
	out := cardView(t, deps)
	wantIn(t, out, "0 goals · 0 assists")
	if strings.Contains(out, "clean sheets") {
		t.Errorf("no detail, no clean sheets\n%s", out)
	}
}

func TestPlayerCardRefreshing(t *testing.T) {
	_, deps := starPlayer(t, models.Player{ID: "p-raya", Name: "David Raya", Position: "GK"},
		models.PlayerSeasonStats{Appearances: 37, Minutes: 3330}, nil)
	var s core.Screen = load(t, deps, 120)
	s, _ = s.Update(core.RefreshQueueMsg{Active: &core.RefreshRequest{PlayerID: "p-raya"}})
	if !strings.Contains(ansi.Strip(s.View()), "🔄 refreshing") {
		t.Errorf("the card should say its player is refreshing\n%s", s.View())
	}
}
