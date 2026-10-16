package dashboard_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/dashboard"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

// intlDashboard stars, in this order: a nation, a club league, a
// tournament.
func intlDashboard(t *testing.T) (*dashboard.Model, storage.Repositories) {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())
	t0 := storagetest.Fixed
	favs := []models.Favorite{
		{Kind: models.FavoriteTeam, ID: "nat-arcadia", AddedAt: t0},
		{Kind: models.FavoriteCompetition, ID: "eng-premier-league", AddedAt: t0.Add(time.Minute)},
		{Kind: models.FavoriteCompetition, ID: "world-cup", AddedAt: t0.Add(2 * time.Minute)},
	}
	for _, f := range favs {
		if err := repos.AddFavorite(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(favs)}
	s := tuitest.Resize(t, tuitest.Settle(t, dashboard.New(deps)), 120, 60)
	return s.(*dashboard.Model), repos
}

func TestGroupsRunLeaguesTournamentsClubsNationsPlayers(t *testing.T) {
	m, _ := intlDashboard(t)
	var groups []dashboard.Group
	for _, c := range m.Cards() {
		groups = append(groups, c.Group)
	}
	want := []dashboard.Group{dashboard.GroupLeagues, dashboard.GroupTournaments, dashboard.GroupNations}
	if len(groups) != 3 || groups[0] != want[0] || groups[1] != want[1] || groups[2] != want[2] {
		t.Fatalf("groups %v, want %v", groups, want)
	}
	out := m.View()
	l, tn, n := strings.Index(out, "LEAGUES"), strings.Index(out, "TOURNAMENTS"), strings.Index(out, "NATIONS")
	if l < 0 || tn < l || n < tn {
		t.Errorf("headings out of order:\n%s", out)
	}
}

// After the group stage, a tournament card shows the latest round.
func TestATournamentCardShowsTheLatestRoundOnceKnockoutsBegin(t *testing.T) {
	m, _ := intlDashboard(t)
	card := m.Cards()[1]
	if card.RoundLabel != "Quarterfinal" || len(card.Results) != 1 {
		t.Errorf("card %+v", card)
	}
	if !strings.Contains(m.View(), "Arcadia 1–1 (3–4 p) Carpania") {
		t.Errorf("the quarterfinal should be on the card:\n%s", m.View())
	}
}

// During the group stage it shows the starred nation's group.
func TestATournamentCardShowsAStarredNationsGroupDuringTheGroupStage(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	// The same tournament, before the knockouts: the quarterfinal hasn't
	// been played yet.
	ms := storagetest.SampleMatches()
	qf := ms[6]
	qf.Status, qf.HomeScore, qf.AwayScore, qf.HomePens, qf.AwayPens = models.MatchScheduled, nil, nil, nil, nil
	if err := repos.UpsertMatches(ctx, []models.Match{qf}); err != nil {
		t.Fatal(err)
	}
	favs := []models.Favorite{{Kind: models.FavoriteCompetition, ID: "world-cup", AddedAt: storagetest.Fixed},
		{Kind: models.FavoriteTeam, ID: "nat-borduria", AddedAt: storagetest.Fixed}}
	for _, f := range favs {
		_ = repos.AddFavorite(ctx, f)
	}
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(favs)}
	m := tuitest.Settle(t, dashboard.New(deps)).(*dashboard.Model)
	card := m.Cards()[0]
	if card.GroupLabel != "Group K" || len(card.Table) != 2 || card.Table[1].Team.ID != "nat-borduria" {
		t.Errorf("card %+v, want Borduria's group", card)
	}
}

func TestANationCardShowsResultsNextFixtureAndForm(t *testing.T) {
	m, _ := intlDashboard(t)
	card := m.Cards()[2]
	if len(card.Recent) != 3 || card.Recent[0].ID != "m07" || card.Next == nil || card.Next.ID != "m08" {
		t.Errorf("recent %v, next %v", card.Recent, card.Next)
	}
	if card.Summary.Played != 7 || card.Summary.Form != "DWWLD" {
		t.Errorf("summary %+v", card.Summary)
	}
	out := m.View()
	for _, want := range []string{"★ Arcadia", "national team · men · all matches", "P7 W4 D2 L1  DWWLD", "next:", "v Borduria"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q\n%s", want, out)
		}
	}
}

// The card counts what the nation screen's saved filter counts.
func TestANationCardFollowsTheSavedFilter(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	if err := repos.SetTeamMatchFilter(ctx, "nat-arcadia", models.MatchFilter{Preset: models.PresetCompetitive}); err != nil {
		t.Fatal(err)
	}
	favs := []models.Favorite{{Kind: models.FavoriteTeam, ID: "nat-arcadia", AddedAt: storagetest.Fixed}}
	_ = repos.AddFavorite(ctx, favs[0])
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(favs)}
	m := tuitest.Settle(t, dashboard.New(deps)).(*dashboard.Model)
	card := m.Cards()[0]
	if card.Summary.Played != 5 || card.Next != nil || !strings.Contains(m.View(), "competitive") {
		t.Errorf("card %+v: the friendly fixture isn't competitive", card.Summary)
	}
}

func TestANationCardRefreshesItsMatchesAndSaysSo(t *testing.T) {
	m, _ := intlDashboard(t)
	m = tuitest.Key(t, m, "end").(*dashboard.Model)
	req, ok := m.RefreshRequest()
	if !ok || req.TeamID != "nat-arcadia" {
		t.Fatalf("request %+v", req)
	}
	if _, ok := m.RefreshTarget(); ok {
		t.Error("a nation card has no single competition to refresh")
	}
	updated, _ := m.Update(core.RefreshQueueMsg{Pending: []core.RefreshRequest{{Nations: "men"}}})
	if !strings.Contains(updated.View(), "queued for refresh") {
		t.Error("a queued gender-wide refresh covers the nation's card")
	}
}
