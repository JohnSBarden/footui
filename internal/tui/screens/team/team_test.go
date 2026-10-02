package team_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/trophies"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/team"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

var arsenal = models.Team{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}

func newDeps(repos storage.Repositories) core.Deps {
	return core.Deps{Ctx: context.Background(), Repos: repos, Theme: styles.Plain()}
}

func settled(t *testing.T, deps core.Deps, competitionID, seasonID string) core.Screen {
	t.Helper()
	s := tuitest.Settle(t, team.New(deps, arsenal, competitionID, seasonID))
	return tuitest.Resize(t, s, 110, 44)
}

// seedTwoCompetitions gives Arsenal the same 2025 season in two different
// competitions with deliberately different numbers, which is what the
// no-blending assertions rely on.
func seedTwoCompetitions(t *testing.T) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)

	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: arsenal.ID, Name: "Arsenal", Position: 1, MP: 38, W: 26, D: 7, L: 5, GF: 71, GA: 27, Pts: 85, XG: 77.5, XGA: 33.1},
	})
	storagetest.SeedCompetition(t, repos, storagetest.ChampionsLeague(), "2025", []storagetest.TeamRow{
		{ID: arsenal.ID, Name: "Arsenal", Position: 3, MP: 13, W: 8, D: 2, L: 3, GF: 24, GA: 11, Pts: 26, XG: 21.4, XGA: 13.9},
	})
	return repos
}

func TestTeamStatsAreNotBlendedAcrossCompetitions(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025")

	league := s.View()
	// The Premier League tab must show only the league's 38 games and 85
	// points, never 51 games or 111 points.
	if !strings.Contains(league, "85") || !strings.Contains(league, "38") {
		t.Errorf("Premier League tab missing its own figures\n---\n%s", league)
	}
	if strings.Contains(league, "111") || strings.Contains(league, "51 played") {
		t.Errorf("figures were blended across competitions\n---\n%s", league)
	}

	// Switching tabs must show the other competition's distinct numbers.
	s = tuitest.Key(t, s, "right")
	cup := s.View()
	if !strings.Contains(cup, "26") || !strings.Contains(cup, "13 played") {
		t.Errorf("Champions League tab missing its own figures\n---\n%s", cup)
	}
	if strings.Contains(cup, "85") {
		t.Errorf("Premier League points leaked into the Champions League tab\n---\n%s", cup)
	}
}

func TestBothCompetitionsAppearAsTabs(t *testing.T) {
	repos := seedTwoCompetitions(t)
	out := settled(t, newDeps(repos), "", "").View()

	for _, want := range []string{"Premier League", "Champions League"} {
		if !strings.Contains(out, want) {
			t.Errorf("tab bar missing %q\n---\n%s", want, out)
		}
	}
}

func TestOpensOnTheCompetitionAndSeasonDrilledInFrom(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "uefa-champions-league", "uefa-champions-league:2025")

	m := s.(*team.Model)
	comp, ok := m.CurrentCompetition()
	if !ok {
		t.Fatal("no competition selected")
	}
	if comp.ID != "uefa-champions-league" {
		t.Errorf("opened on %q, want the competition drilled in from", comp.ID)
	}
}

func TestDefaultsToFirstCompetitionWhenOpenedWithoutContext(t *testing.T) {
	repos := seedTwoCompetitions(t)
	s := settled(t, newDeps(repos), "", "")

	m := s.(*team.Model)
	comp, ok := m.CurrentCompetition()
	if !ok {
		t.Fatal("no competition selected")
	}
	// The domestic league leads: a continental cup has no tier, and
	// unknown tiers sort after real ones.
	if comp.Name != "Premier League" {
		t.Errorf("defaulted to %q, want the domestic league as the first tab", comp.Name)
	}
}

func TestSquadLoadsForTheSelectedSeason(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	for _, want := range []string{"Squad", "Bukayo Saka", "Declan Rice", "David Raya"} {
		if !strings.Contains(out, want) {
			t.Errorf("squad table missing %q\n---\n%s", want, out)
		}
	}
}

func TestSwitchingCompetitionSwapsTheSquadRatherThanKeepingAStaleOne(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())
	storagetest.SeedRoster(t, repos, arsenal.ID, "uefa-champions-league", "uefa-champions-league:2025", []storagetest.PlayerRow{
		{ID: "p-havertz", Name: "Kai Havertz", Position: "F S", Apps: 11, Minutes: 800, Goals: 5, Assists: 1, XG: 4.2, XA: 1.1},
	})

	s := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025")
	if out := s.View(); !strings.Contains(out, "Bukayo Saka") {
		t.Fatalf("expected the league squad first\n---\n%s", out)
	}

	s = tuitest.Key(t, s, "right")
	out := s.View()
	if !strings.Contains(out, "Kai Havertz") {
		t.Errorf("expected the cup squad after switching tabs\n---\n%s", out)
	}
	if strings.Contains(out, "Bukayo Saka") {
		t.Errorf("stale squad from the previous competition still shown\n---\n%s", out)
	}
}

func TestTabMovesFocusToSquadAndEnterOpensThePlayer(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())

	var opened models.Player
	deps := newDeps(repos)
	deps.Nav.Player = func(_ core.Deps, p models.Player, _ string) core.Screen {
		opened = p
		return stubScreen{}
	}

	s := settled(t, deps, "eng-premier-league", "eng-premier-league:2025")
	s = tuitest.Key(t, s, "tab")
	s = tuitest.Key(t, s, "down") // second squad row

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatalf("expected enter on the squad pane to push the Player screen")
	}
	// The squad is ordered by minutes descending, so row two is Rice.
	if opened.Name != "Declan Rice" {
		t.Errorf("opened %q, want the player under the cursor", opened.Name)
	}
}

func TestTabDoesNotFocusAnEmptySquad(t *testing.T) {
	repos := seedTwoCompetitions(t) // deliberately no roster seeded

	var openedCompetition string
	deps := newDeps(repos)
	deps.Nav.League = func(_ core.Deps, competitionID, _ string) core.Screen {
		openedCompetition = competitionID
		return stubScreen{}
	}

	s := settled(t, deps, "eng-premier-league", "eng-premier-league:2025")
	s = tuitest.Key(t, s, "tab")

	// Focus should have stayed on Seasons, so enter opens the league
	// table rather than a player that doesn't exist.
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tuitest.CapturedCmd(cmd).(core.PushMsg); !ok {
		t.Fatal("expected enter to push a screen")
	}
	if openedCompetition != "eng-premier-league" {
		t.Errorf("enter opened %q; with no squad, focus must stay on the seasons pane", openedCompetition)
	}
}

func TestEmptyStateWhenTeamHasNoCachedSeasons(t *testing.T) {
	repos := storagetest.New(t)
	out := settled(t, newDeps(repos), "", "").View()

	if !strings.Contains(out, "No cached seasons") {
		t.Errorf("expected an empty state\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("empty state must name the command that fixes it\n---\n%s", out)
	}
}

func TestStatCardsShowExpectedGoalsGap(t *testing.T) {
	repos := seedTwoCompetitions(t)
	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()

	// Arsenal scored 71 against an xG of 77.5, i.e. 6.5 fewer than expected.
	if !strings.Contains(out, "-6.5 vs GF") {
		t.Errorf("expected the goals-vs-xG gap on a stat card\n---\n%s", out)
	}
}

func TestAdvancedTeamStatsRenderAsCards(t *testing.T) {
	repos := seedTwoCompetitions(t)
	ctx := context.Background()
	// Arsenal took 85 points against 79.9 expected: a season that
	// outperformed its chances by 5.1.
	if err := repos.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{
		TeamID: arsenal.ID, CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		MatchesPlayed: 38, Wins: 26, Draws: 7, Losses: 5, GoalsFor: 71, GoalsAgainst: 27,
		GoalDiff: 44, Points: 85, Source: "test", UpdatedAt: storagetest.Fixed,
		Extras: map[string]float64{
			models.StatExpectedPoints: 79.9,
			models.StatPPDA:           9.6,
			models.StatNPxG:           74.4,
			models.StatDeep:           349,
			models.StatDeepAllowed:    142,
		},
	}); err != nil {
		t.Fatalf("UpsertTeamSeasonStats: %v", err)
	}

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()

	for _, want := range []string{"xPts", "79.9", "+5.1 vs Pts", "PPDA", "9.6", "npxG", "Deep", "142 allowed"} {
		if !strings.Contains(out, want) {
			t.Errorf("advanced stat cards missing %q\n---\n%s", want, out)
		}
	}
}

// A provider with no long-tail stats must produce no cards at all, rather
// than a row of cards reading zero.
func TestNoAdvancedStatsMeansNoAdvancedCards(t *testing.T) {
	repos := seedTwoCompetitions(t) // seeded without extras
	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()

	for _, absent := range []string{"xPts", "PPDA", "npxG", "Deep"} {
		if strings.Contains(out, absent) {
			t.Errorf("rendered a %q card with no data behind it\n---\n%s", absent, out)
		}
	}
}

func TestSquadShowsShotsAndKeyPasses(t *testing.T) {
	repos := seedTwoCompetitions(t)
	ctx := context.Background()
	if err := repos.UpsertPlayer(ctx, models.Player{ID: "p-saka", Name: "Bukayo Saka", Position: "F M S", Gender: "men"}); err != nil {
		t.Fatalf("UpsertPlayer: %v", err)
	}
	shots, keyPasses := 71, 62
	if err := repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
		PlayerID: "p-saka", TeamID: arsenal.ID,
		CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Appearances: 31, Minutes: 2239, Goals: 7, Assists: 5,
		Shots: &shots, KeyPasses: &keyPasses,
		Source: "test", UpdatedAt: storagetest.Fixed,
	}); err != nil {
		t.Fatalf("UpsertPlayerSeasonStats: %v", err)
	}

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	if !strings.Contains(out, "Sh") || !strings.Contains(out, "KP") {
		t.Errorf("squad table missing shots/key-passes columns\n---\n%s", out)
	}
	if !strings.Contains(out, "71") || !strings.Contains(out, "62") {
		t.Errorf("squad table missing the values\n---\n%s", out)
	}
}

func TestClubColorsThemeTheScreen(t *testing.T) {
	repos := seedTwoCompetitions(t)
	ctx := context.Background()
	if err := repos.UpsertTeamColors(ctx, models.TeamColors{
		TeamID: arsenal.ID, PrimaryHex: "#EF0107", SecondaryHex: "#023474", Source: "test",
	}); err != nil {
		t.Fatalf("UpsertTeamColors: %v", err)
	}

	deps := newDeps(repos)
	deps.Theme = styles.Default() // a color-capable theme
	s := tuitest.Settle(t, team.New(deps, arsenal, "eng-premier-league", "eng-premier-league:2025"))

	if !s.(*team.Model).Themed() {
		t.Error("a club with seeded colors should render in its own theme")
	}
}

func TestStrongModePaintsTheClubAndTogglesBack(t *testing.T) {
	repos := seedTwoCompetitions(t)
	if err := repos.UpsertTeamColors(context.Background(), models.TeamColors{
		TeamID: arsenal.ID, PrimaryHex: "#EF0107", SecondaryHex: "#FFFFFF", Source: "test",
	}); err != nil {
		t.Fatalf("UpsertTeamColors: %v", err)
	}

	deps := newDeps(repos)
	deps.Theme = styles.Default()
	deps.Appearance = &styles.Appearance{Mode: styles.Strong}
	s := tuitest.Settle(t, team.New(deps, arsenal, "eng-premier-league", "eng-premier-league:2025")).(*team.Model)

	if s.ScreenTheme().Background == nil {
		t.Fatal("strong mode should give the club screen a background")
	}

	s.Update(core.ThemeChangedMsg{Mode: styles.Weak})
	if s.ScreenTheme().Background != nil {
		t.Error("switching to weak should drop the background")
	}
	if !s.Themed() {
		t.Error("weak mode is still the club's colors")
	}
}

// Uncurated clubs are the common case and must render normally.
func TestClubWithoutColorsStaysOnTheNeutralTheme(t *testing.T) {
	deps := newDeps(seedTwoCompetitions(t))
	deps.Theme = styles.Default()

	s := tuitest.Settle(t, team.New(deps, arsenal, "eng-premier-league", "eng-premier-league:2025"))
	if s.(*team.Model).Themed() {
		t.Error("a club with no seeded colors should not claim to be branded")
	}
	if s.View() == "" {
		t.Error("an uncurated club must still render")
	}
}

// NO_COLOR means no color; a club's branding is not an exception.
func TestNoColorSuppressesClubBranding(t *testing.T) {
	repos := seedTwoCompetitions(t)
	if err := repos.UpsertTeamColors(context.Background(), models.TeamColors{
		TeamID: arsenal.ID, PrimaryHex: "#EF0107", Source: "test",
	}); err != nil {
		t.Fatalf("UpsertTeamColors: %v", err)
	}

	deps := newDeps(repos) // styles.Plain()
	s := tuitest.Settle(t, team.New(deps, arsenal, "eng-premier-league", "eng-premier-league:2025"))

	if s.(*team.Model).Themed() {
		t.Error("club colors must not override the plain theme")
	}
}

// Branding belongs to the screen it's on: a player who turned out for
// three clubs should not be rendered in one club's colors.
func TestClubBrandingDoesNotLeakIntoPushedScreens(t *testing.T) {
	repos := seedTwoCompetitions(t)
	ctx := context.Background()
	if err := repos.UpsertTeamColors(ctx, models.TeamColors{
		TeamID: arsenal.ID, PrimaryHex: "#EF0107", Source: "test",
	}); err != nil {
		t.Fatalf("UpsertTeamColors: %v", err)
	}
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025", storagetest.SampleSquad())

	var childTheme styles.Theme
	deps := newDeps(repos)
	deps.Theme = styles.Default()
	deps.Nav.Player = func(d core.Deps, _ models.Player, _ string) core.Screen {
		childTheme = d.Theme
		return stubScreen{}
	}

	s := tuitest.Resize(t, tuitest.Settle(t, team.New(deps, arsenal, "eng-premier-league", "eng-premier-league:2025")), 110, 44)
	if !s.(*team.Model).Themed() {
		t.Fatal("expected the team screen to be branded")
	}

	s = tuitest.Key(t, s, "tab")
	s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if _, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		tuitest.CapturedCmd(cmd)
	}

	if childTheme.Palette.Accent != styles.DefaultPalette().Accent {
		t.Error("a pushed screen should inherit the neutral theme, not the club's colors")
	}
}

func seedHonours(t *testing.T, repos storage.Repositories, teamID string, rows ...models.Trophy) {
	t.Helper()
	ctx := context.Background()
	for _, tr := range rows {
		tr.TeamID = teamID
		if err := repos.InsertTrophy(ctx, tr); err != nil {
			t.Fatalf("InsertTrophy: %v", err)
		}
	}
}

func TestHonoursPanelAggregatesRepeatedTitles(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedHonours(t, repos, arsenal.ID,
		models.Trophy{Title: "Premier League", SeasonLabel: "2001-02", TrophyType: "league_title", Source: "test"},
		models.Trophy{Title: "Premier League", SeasonLabel: "2003-04", TrophyType: "league_title", Source: "test"},
		models.Trophy{Title: "Premier League", SeasonLabel: "2025-26", TrophyType: "league_title", Source: "test"},
	)

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()

	if !strings.Contains(out, "Honours") {
		t.Fatalf("expected an honours panel\n---\n%s", out)
	}
	// Three stored rows read as one line with a count, not three lines.
	if !strings.Contains(out, "Premier League ×3") {
		t.Errorf("expected repeated titles aggregated with a count\n---\n%s", out)
	}
	if !strings.Contains(out, "2025-26, 2003-04, 2001-02") {
		t.Errorf("expected seasons listed most recent first\n---\n%s", out)
	}
}

// The curated league-title lists start at a fixed season, so a count on
// screen must never be readable as an all-time total.
func TestHonoursPanelStatesItsCoverageLimit(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedHonours(t, repos, arsenal.ID,
		models.Trophy{Title: "Premier League", SeasonLabel: "2003-04", TrophyType: "league_title", Source: "test"})

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	if !strings.Contains(out, "league titles from "+trophies.LeagueTitlesFrom) {
		t.Errorf("honours panel must state the dataset's cutoff\n---\n%s", out)
	}
}

func TestContinentalHonoursAreListedFirst(t *testing.T) {
	repos := seedTwoCompetitions(t)
	seedHonours(t, repos, arsenal.ID,
		models.Trophy{Title: "Premier League", SeasonLabel: "2003-04", TrophyType: "league_title", Source: "test"},
		models.Trophy{Title: "European Cup", SeasonLabel: "1993-94", TrophyType: "continental_cup", Source: "test"},
	)

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	panel := honoursPanel(t, out)

	european := strings.Index(panel, "European Cup")
	league := strings.Index(panel, "Premier League")
	if european < 0 || league < 0 || european > league {
		t.Errorf("continental honours should lead the panel\n---\n%s", panel)
	}
}

// honoursPanel isolates the honours section, since competition names also
// appear in the tab bar higher up the view.
func honoursPanel(t *testing.T, view string) string {
	t.Helper()
	i := strings.Index(view, "Honours")
	if i < 0 {
		t.Fatalf("no honours panel in view\n---\n%s", view)
	}
	return view[i:]
}

// An empty panel would imply the club has won nothing, which is a
// different and probably false claim from "not in the curated dataset".
func TestUncuratedClubShowsNoHonoursPanelAtAll(t *testing.T) {
	out := settled(t, newDeps(seedTwoCompetitions(t)), "eng-premier-league", "eng-premier-league:2025").View()

	if strings.Contains(out, "Honours") {
		t.Errorf("a club with no curated honours should show no panel\n---\n%s", out)
	}
}

func TestLongHonoursListsAreSummarized(t *testing.T) {
	repos := seedTwoCompetitions(t)
	var rows []models.Trophy
	for year := 2005; year <= 2025; year++ {
		rows = append(rows, models.Trophy{
			Title:       "Premier League",
			SeasonLabel: strconv.Itoa(year) + "-" + strconv.Itoa((year+1)%100),
			TrophyType:  "league_title", Source: "test",
		})
	}
	seedHonours(t, repos, arsenal.ID, rows...)

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	if !strings.Contains(out, "×21") {
		t.Errorf("expected the full count\n---\n%s", out)
	}
	if !strings.Contains(out, "more") {
		t.Errorf("a 21-season list should be summarized rather than wrapping the terminal\n---\n%s", out)
	}
}

// Same rule as the League table: a narrow terminal gives up detail, never
// identity. Two squad members with similar names must stay distinguishable.
func TestNarrowTerminalsKeepPlayerNamesIntact(t *testing.T) {
	repos := seedTwoCompetitions(t)
	storagetest.SeedRoster(t, repos, arsenal.ID, "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{
			{ID: "p-zub", Name: "Martín Zubimendi", Position: "D M S", Apps: 38, Minutes: 3040, Goals: 5, XG: 2.9},
			{ID: "p-zin", Name: "Martín Zubizarreta", Position: "D M S", Apps: 20, Minutes: 1500, Goals: 1, XG: 0.8},
		})

	for _, width := range []int{60, 70, 80, 100, 130} {
		s := tuitest.Settle(t, team.New(newDeps(repos), arsenal, "eng-premier-league", "eng-premier-league:2025"))
		out := tuitest.Resize(t, s, width, 44).View()

		for _, name := range []string{"Martín Zubimendi", "Martín Zubizarreta"} {
			if !strings.Contains(out, name) {
				t.Errorf("width %d truncated %q\n---\n%s", width, name, out)
			}
		}
		// The columns the table exists for are never sacrificed.
		for _, col := range []string{"Apps", "Mins", "G", "A"} {
			if !strings.Contains(out, col) {
				t.Errorf("width %d dropped the core column %q\n---\n%s", width, col, out)
			}
		}
	}
}

func TestTeamScreenShowsWhenTheSeasonWasLastRefreshed(t *testing.T) {
	repos := seedTwoCompetitions(t)
	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{
		Provider: "understat", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		StartedAt: time.Now().Add(-4 * time.Hour), FinishedAt: time.Now().Add(-4 * time.Hour),
		Status: models.SyncStatusOK,
	}); err != nil {
		t.Fatalf("RecordSyncRun: %v", err)
	}

	out := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025").View()
	if !strings.Contains(out, "updated 4 hours ago") {
		t.Errorf("expected a freshness indicator\n---\n%s", out)
	}
}

// Freshness is per competition-season, so switching tabs must not carry
// the previous tab's timestamp across.
func TestFreshnessFollowsTheSelectedCompetition(t *testing.T) {
	repos := seedTwoCompetitions(t)
	ctx := context.Background()
	if err := repos.RecordSyncRun(ctx, models.SyncRun{
		Provider: "understat", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		StartedAt: time.Now().Add(-4 * time.Hour), FinishedAt: time.Now().Add(-4 * time.Hour),
		Status: models.SyncStatusOK,
	}); err != nil {
		t.Fatalf("RecordSyncRun: %v", err)
	}

	s := settled(t, newDeps(repos), "eng-premier-league", "eng-premier-league:2025")
	if !strings.Contains(s.View(), "updated 4 hours ago") {
		t.Fatal("setup: expected the league tab to show its freshness")
	}

	// The cup tab has no sync log entry of its own.
	s = tuitest.Key(t, s, "right")
	if out := s.View(); strings.Contains(out, "4 hours ago") {
		t.Errorf("the previous tab's freshness leaked across\n---\n%s", out)
	}
}

func TestRefreshTargetIsTheSelectedCompetitionTab(t *testing.T) {
	s := settled(t, newDeps(seedTwoCompetitions(t)), "eng-premier-league", "eng-premier-league:2025")
	got, ok := s.(core.Refreshable).RefreshTarget()
	if !ok || got != "eng-premier-league" {
		t.Errorf("RefreshTarget = %q, %v; want the selected competition", got, ok)
	}
}

// With nothing cached there is no tab to read, but a team opened from a
// league still knows which one it came from.
func TestRefreshTargetFallsBackToTheOriginCompetition(t *testing.T) {
	s := settled(t, newDeps(storagetest.New(t)), "esp-la-liga", "")
	got, ok := s.(core.Refreshable).RefreshTarget()
	if !ok || got != "esp-la-liga" {
		t.Errorf("RefreshTarget = %q, %v; want the competition the team was opened from", got, ok)
	}
	if got, ok := settled(t, newDeps(storagetest.New(t)), "", "").(core.Refreshable).RefreshTarget(); ok {
		t.Errorf("RefreshTarget = %q; with no history and no origin there is nothing to refresh", got)
	}
}

func TestEmptyStateOffersTheRefreshKeyOnlyWhenThereIsATarget(t *testing.T) {
	if out := settled(t, newDeps(storagetest.New(t)), "esp-la-liga", "").View(); !strings.Contains(out, "Press  r") {
		t.Errorf("a team opened from a league should offer the refresh key\n---\n%s", out)
	}
	if out := settled(t, newDeps(storagetest.New(t)), "", "").View(); strings.Contains(out, "Press  r") {
		t.Errorf("a team with no competition to refresh must not offer a key that does nothing\n---\n%s", out)
	}
}

func TestRefreshedMsgReloadsTheHistory(t *testing.T) {
	repos := storagetest.New(t)
	s := settled(t, newDeps(repos), "eng-premier-league", "")
	if !strings.Contains(s.View(), "No cached seasons") {
		t.Fatalf("precondition: expected the empty state\n---\n%s", s.View())
	}

	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: arsenal.ID, Name: "Arsenal", Position: 1, MP: 38, Pts: 85},
	})
	updated, cmd := s.Update(core.RefreshedMsg{CompetitionID: "eng-premier-league"})
	s = tuitest.Resize(t, tuitest.Drive(t, updated, cmd), 110, 44)

	if strings.Contains(s.View(), "No cached seasons") {
		t.Errorf("history should show after the refresh\n---\n%s", s.View())
	}
}
