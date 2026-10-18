package dashboard_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/dashboard"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var ctx = context.Background()

func init() { dashboard.PinClock(storagetest.Fixed) }

type stub struct{ name string }

func (stub) Init() tea.Cmd                           { return nil }
func (s stub) Update(tea.Msg) (core.Screen, tea.Cmd) { return s, nil }
func (s stub) View() string                          { return s.name }
func (s stub) Title() string                         { return s.name }
func (stub) Keys() core.KeyMap                       { return core.KeyMap{} }

// opened records what the dashboard asked to open.
type opened struct {
	kind, id, comp, season, team string
}

// seed caches a Premier League season with a squad, and stars, in this
// order: a player, a club, the league, and a club that isn't cached.
func seed(t *testing.T) (storage.Repositories, core.Deps, *opened) {
	t.Helper()
	repos := storagetest.New(t)
	seasonID := storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())
	storagetest.SeedRoster(t, repos, "eng-arsenal", "eng-premier-league", seasonID, storagetest.SampleSquad())
	if err := repos.RecordSyncRun(ctx, models.SyncRun{Provider: "test", CompetitionID: "eng-premier-league",
		SeasonID: seasonID, StartedAt: storagetest.Fixed, FinishedAt: storagetest.Fixed.Add(-3 * time.Hour),
		Status: models.SyncStatusOK}); err != nil {
		t.Fatal(err)
	}

	t0 := storagetest.Fixed
	favs := []models.Favorite{
		{Kind: models.FavoritePlayer, ID: "p-saka", AddedAt: t0},
		{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: t0.Add(time.Minute)},
		{Kind: models.FavoriteCompetition, ID: "eng-premier-league", AddedAt: t0.Add(2 * time.Minute)},
		{Kind: models.FavoriteTeam, ID: "eng-gone", AddedAt: t0.Add(3 * time.Minute)},
	}
	for _, f := range favs {
		if err := repos.AddFavorite(ctx, f); err != nil {
			t.Fatal(err)
		}
	}

	got := &opened{}
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(favs)}
	deps.Nav = core.Navigator{
		League: func(_ core.Deps, id, season string) core.Screen {
			*got = opened{kind: "league", id: id, season: season}
			return stub{"league"}
		},
		Team: func(_ core.Deps, team models.Team, comp, season string) core.Screen {
			*got = opened{kind: "team", id: team.ID, comp: comp, season: season}
			return stub{"team"}
		},
		Player: func(_ core.Deps, p models.Player, teamID string) core.Screen {
			*got = opened{kind: "player", id: p.ID, team: teamID}
			return stub{"player"}
		},
		Browse: func(core.Deps, string) core.Screen { return stub{"browse"} },
	}
	return repos, deps, got
}

func load(t *testing.T, deps core.Deps, width int) *dashboard.Model {
	t.Helper()
	s := tuitest.Settle(t, dashboard.New(deps))
	s = tuitest.Resize(t, s, width, 40)
	return s.(*dashboard.Model)
}

func TestCardsAreGroupedLeaguesClubsPlayers(t *testing.T) {
	_, deps, _ := seed(t)
	m := load(t, deps, 120)

	var got []string
	for _, c := range m.Cards() {
		got = append(got, string(c.Kind)+":"+c.ID)
	}
	want := "competition:eng-premier-league team:eng-arsenal team:eng-gone player:p-saka"
	if strings.Join(got, " ") != want {
		t.Fatalf("order %v, want %s", got, want)
	}
	if !m.Cards()[2].Missing {
		t.Error("a favourite that isn't cached should be a Missing card")
	}
}

// A starred club's card shows its form over its last games in every
// competition, oldest first.
func TestAClubCardShowsItsForm(t *testing.T) {
	repos, deps, _ := seed(t)
	day := func(d int) time.Time { return storagetest.Fixed.AddDate(0, 0, -d) }
	one, two, zero := 1, 2, 0
	if err := repos.UpsertMatches(ctx, []models.Match{
		{ID: "m1", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025", Kickoff: day(7),
			Status: models.MatchFullTime, HomeTeamID: "eng-arsenal", AwayTeamID: "eng-luton-town",
			HomeScore: &zero, AwayScore: &one, Source: "test", UpdatedAt: day(0)},
		{ID: "m2", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025", Kickoff: day(3),
			Status: models.MatchFullTime, HomeTeamID: "eng-manchester-city", AwayTeamID: "eng-arsenal",
			HomeScore: &one, AwayScore: &two, Source: "test", UpdatedAt: day(0)},
	}); err != nil {
		t.Fatal(err)
	}
	// Beside the headline, where there's room.
	view := load(t, deps, 120).View()
	if !regexp.MustCompile(`1st · 90 pts +L W`).MatchString(view) {
		t.Errorf("no form beside the headline:\n%s", view)
	}
}

func TestCardsShowWhereEachFavouriteStands(t *testing.T) {
	_, deps, _ := seed(t)
	view := load(t, deps, 120).View()

	for _, want := range []string{
		"▦ Premier League", "England · 2025-26", // a league's badge is a table
		"▌ Arsenal",           // a club's is its colours, as halves
		"★ Arsenal",           // the starred club in the league table
		"1st · 90 pts",        // the club card's headline
		"P38  W28  D6  L4",    // its record
		"84.2",                // xG, drawn against the 91 scored
		"◉ Bukayo Saka",       // the player card
		"7 goals · 5 assists", // their season
		"xG 8.7 · xA 8.6",     // with understat's expected numbers
		"Not cached (team).",  // the missing club
		"3h │",                // freshness, at the heading's right
	} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard is missing %q:\n%s", want, view)
		}
	}
}

func TestArrowKeysMoveAcrossTheGrid(t *testing.T) {
	_, deps, _ := seed(t)
	// 120 columns hold three: the league spans the first two, and the
	// other cards stack down the third beside it.
	var s core.Screen = load(t, deps, 120)
	sel := func() string { c, _ := s.(*dashboard.Model).Selected(); return c.ID }

	steps := []struct{ key, want string }{
		{"down", "eng-premier-league"}, // nothing under the league
		{"right", "eng-arsenal"},       // level with its top
		{"down", "eng-gone"},
		{"down", "p-saka"},
		{"down", "p-saka"}, // stays put at the bottom
		{"left", "eng-premier-league"},
		{"up", "eng-premier-league"},
		{"G", "p-saka"},
		{"up", "eng-gone"},
		{"g", "eng-premier-league"},
	}
	for _, st := range steps {
		s = tuitest.Key(t, s, st.key)
		if got := sel(); got != st.want {
			t.Fatalf("after %s: on %s, want %s", st.key, got, st.want)
		}
	}
}

func TestEnterOpensTheCardsScreenWithItsSeason(t *testing.T) {
	_, deps, got := seed(t)
	m := load(t, deps, 120)

	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("enter should push a screen")
	}
	if *got != (opened{kind: "team", id: "eng-arsenal", comp: "eng-premier-league", season: "eng-premier-league:2025"}) {
		t.Errorf("opened %+v", *got)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tuitest.CapturedCmd(cmd)
	if *got != (opened{kind: "player", id: "p-saka", team: "eng-arsenal"}) {
		t.Errorf("opened %+v", *got)
	}

	// A missing card explains itself rather than opening an empty screen.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if msg, ok := tuitest.CapturedCmd(cmd).(core.StatusMsg); !ok || !strings.Contains(msg.Text, "isn't cached") {
		t.Errorf("enter on a missing card gave %#v", msg)
	}
}

func TestSelectedCardIsTheFavouriteAndRefreshTarget(t *testing.T) {
	_, deps, _ := seed(t)
	m := load(t, deps, 120)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})

	target, ok := m.FavoriteTarget()
	if !ok || target != (core.FavoriteTarget{Kind: models.FavoritePlayer, ID: "p-saka", Name: "Bukayo Saka"}) {
		t.Errorf("favourite target %+v", target)
	}
	if comp, ok := m.RefreshTarget(); !ok || comp != "eng-premier-league" {
		t.Errorf("refresh target %q, %v", comp, ok)
	}
}

func TestUnstarringReloadsTheCards(t *testing.T) {
	repos, deps, _ := seed(t)
	var s core.Screen = load(t, deps, 120)
	if err := repos.RemoveFavorite(ctx, models.FavoriteTeam, "eng-gone"); err != nil {
		t.Fatal(err)
	}
	deps.Favorites.Set(models.FavoriteTeam, "eng-gone", false)
	s = tuitest.Drive(t, s, func() tea.Msg { return core.FavoritesChangedMsg{} })
	if n := len(s.(*dashboard.Model).Cards()); n != 3 {
		t.Errorf("%d cards after un-starring one of four", n)
	}
}

// A favourite club outside the top five is still shown in its league.
func TestLeagueCardShowsAStarredClubOutsideTheTopRows(t *testing.T) {
	repos := storagetest.New(t)
	rows := []storagetest.TeamRow{}
	for i, name := range []string{"A", "B", "C", "D", "E", "F", "Leeds United"} {
		rows = append(rows, storagetest.TeamRow{ID: "t-" + name, Name: name, Position: i + 1, MP: 7, Pts: 20 - i})
	}
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2026", rows)
	favs := []models.Favorite{
		{Kind: models.FavoriteCompetition, ID: "eng-premier-league", AddedAt: storagetest.Fixed},
		{Kind: models.FavoriteTeam, ID: "t-Leeds United", AddedAt: storagetest.Fixed},
	}
	for _, f := range favs {
		if err := repos.AddFavorite(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(favs)}
	view := load(t, deps, 120).View()
	if !strings.Contains(view, "7 ● ★ Leeds United") {
		t.Errorf("the starred club in 7th is missing:\n%s", view)
	}
}

// On a narrow terminal the cards stack in one column and nothing spills
// past the edge.
func TestNarrowTerminalStacksCards(t *testing.T) {
	_, deps, _ := seed(t)
	m := load(t, deps, 40)
	for i, line := range strings.Split(m.View(), "\n") {
		if w := lipgloss.Width(line); w > 40 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
	var s core.Screen = m
	s = tuitest.Key(t, s, "down")
	if c, _ := s.(*dashboard.Model).Selected(); c.ID != "eng-arsenal" {
		t.Errorf("down in one column moved to %s", c.ID)
	}
	s = tuitest.Key(t, s, "down")
	if c, _ := s.(*dashboard.Model).Selected(); c.ID != "eng-gone" {
		t.Errorf("down in one column moved to %s", c.ID)
	}
}

func TestNoFavouritesExplainsHowToAddOne(t *testing.T) {
	repos := storagetest.New(t)
	deps := core.Deps{Ctx: ctx, Repos: repos, Theme: styles.Plain(), Favorites: core.NewFavorites(nil)}
	view := load(t, deps, 120).View()
	if !strings.Contains(view, "No favourites yet.") {
		t.Errorf("empty dashboard:\n%s", view)
	}
}

func TestRefreshQueueShowsOnTheCards(t *testing.T) {
	_, deps, _ := seed(t)
	var s core.Screen = load(t, deps, 120)
	s, _ = s.Update(core.RefreshQueueMsg{Active: &core.RefreshRequest{CompetitionID: "eng-premier-league"}})
	if !strings.Contains(s.View(), "◐ refreshing") {
		t.Error("cards from the competition being refreshed should say so")
	}
}

func TestClubCardShowsXGAndGoalsAsBars(t *testing.T) {
	_, deps, _ := seed(t)
	view := load(t, deps, 120).View()
	for _, want := range []string{"xG    ", "84.2", "goals ", "█", "matchday", "● footui"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
}

func TestClubCardWithoutXGSaysSo(t *testing.T) {
	repos, deps, _ := seed(t)
	luton := models.Favorite{Kind: models.FavoriteTeam, ID: "eng-luton-town", AddedAt: storagetest.Fixed.Add(time.Hour)}
	if err := repos.AddFavorite(ctx, luton); err != nil {
		t.Fatal(err)
	}
	deps.Favorites = core.NewFavorites(append(mustFavorites(t, repos), luton))
	view := load(t, deps, 120).View()
	if !strings.Contains(view, "GF 39 · GA 85 · no xG") {
		t.Errorf("no-xG club card:\n%s", view)
	}
}

func TestStarredClubCardWearsItsScarf(t *testing.T) {
	repos, deps, _ := seed(t)
	if err := repos.UpsertTeamColors(ctx, models.TeamColors{TeamID: "eng-arsenal", PrimaryHex: "#EF0107", SecondaryHex: "#FFFFFF", Source: "test"}); err != nil {
		t.Fatal(err)
	}
	deps.Theme = styles.Default()
	view := lipgloss.NewStyle().Render(load(t, deps, 120).View())
	if !strings.Contains(stripANSI(view), "╭━━━") {
		t.Errorf("no scarf on the Arsenal card:\n%s", stripANSI(view))
	}
}

func TestLeagueCardRowsHaveDots(t *testing.T) {
	_, deps, _ := seed(t)
	view := load(t, deps, 120).View()
	if !strings.Contains(view, "● ★ Arsenal") {
		t.Errorf("league rows should start with a club dot:\n%s", view)
	}
}

func TestDashboardFitsEightyColumns(t *testing.T) {
	_, deps, _ := seed(t)
	for _, l := range strings.Split(load(t, deps, 80).View(), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("overflow %d: %q", lipgloss.Width(l), l)
		}
	}
}

func mustFavorites(t *testing.T, repos storage.Repositories) []models.Favorite {
	t.Helper()
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return favs
}

func stripANSI(s string) string { return ansi.Strip(s) }

// A league's card is twice the others each way: two columns wide, with
// the whole record on each row, and the other cards flow in beside it.
func TestALeagueCardSpansTwoColumnsWithTheWholeRecord(t *testing.T) {
	_, deps, _ := seed(t)
	view := load(t, deps, 120).View()
	lines := strings.Split(view, "\n")
	var head, arsenal string
	for _, l := range lines {
		if strings.Contains(l, "Club") && strings.Contains(l, "Pts") {
			head = l
		}
		if strings.Contains(l, "★ Arsenal") {
			arsenal = l
		}
	}
	for _, col := range []string{"P", "W", "D", "L", "GD"} {
		if !strings.Contains(head, " "+col+" ") {
			t.Errorf("table header %q has no %s column", head, col)
		}
	}
	if !regexp.MustCompile(`★ Arsenal +38 +28 +6 +4 +\+62 .* 90`).MatchString(arsenal) {
		t.Errorf("Arsenal's row %q lacks its record", arsenal)
	}
	// The club card sits beside the league's heading, past its two columns.
	top := lines[3]
	if !strings.Contains(top, "Premier League") || !strings.Contains(top, "Arsenal") {
		t.Errorf("the club card should flow in beside the league:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got > 2+20 {
		t.Errorf("%d lines: the cards should pack beside the league, not under it", got)
	}
}

// On one column a league card is one column wide, like the rest.
func TestANarrowLeagueCardKeepsPointsOnly(t *testing.T) {
	_, deps, _ := seed(t)
	view := load(t, deps, 40).View()
	if strings.Contains(view, "Pts") || !strings.Contains(view, "● ★ Arsenal") {
		t.Errorf("narrow league card:\n%s", view)
	}
}
