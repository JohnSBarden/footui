package league_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/screens/league"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

func newDeps(repos storage.Repositories) core.Deps {
	return core.Deps{
		Ctx:   context.Background(),
		Repos: repos,
		Theme: styles.Plain(),
	}
}

// screen seeds a Premier League season and returns a settled League screen
// laid out at a fixed width, so assertions on rendered output are stable.
func screen(t *testing.T, rows []storagetest.TeamRow) (core.Screen, storage.Repositories) {
	t.Helper()
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", rows)

	s := league.New(newDeps(repos), "eng-premier-league")
	settled := tuitest.Settle(t, s)
	return tuitest.Resize(t, settled, 100, 30), repos
}

func TestViewRendersStandingsInPositionOrder(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())
	out := s.View()

	for _, want := range []string{"Premier League", "England", "tier 1", "2025-26", "Arsenal", "Manchester City", "Pts"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q\n---\n%s", want, out)
		}
	}

	arsenal := strings.Index(out, "Arsenal")
	city := strings.Index(out, "Manchester City")
	luton := strings.Index(out, "Luton Town")
	if !(arsenal < city && city < luton) {
		t.Errorf("rows not in standings order (Arsenal %d, City %d, Luton %d)\n---\n%s", arsenal, city, luton, out)
	}
}

func TestViewShowsGoalDifferenceWithExplicitSign(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())
	out := s.View()

	// Arsenal 91-29 = +62, Wolves 50-65 = -15. The sign has to be visible
	// because under NO_COLOR it's the only thing conveying direction.
	if !strings.Contains(out, "+62") {
		t.Errorf("expected Arsenal's goal difference as +62\n---\n%s", out)
	}
	if !strings.Contains(out, "-15") {
		t.Errorf("expected Wolves' goal difference as -15\n---\n%s", out)
	}
}

func TestViewDistinguishesMissingXGFromZero(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())
	out := s.View()

	if !strings.Contains(out, "—") {
		t.Errorf("a row seeded without xG should render the missing marker, not 0.0\n---\n%s", out)
	}
	if strings.Contains(out, "0.0    0.0") {
		t.Errorf("missing xG rendered as a zero\n---\n%s", out)
	}
}

func TestEmptyStateTellsUserHowToPopulateIt(t *testing.T) {
	repos := storagetest.New(t)
	ctx := context.Background()
	// A competition row exists, but nothing was ever refreshed into it.
	if err := repos.UpsertCompetition(ctx, storagetest.PremierLeague()); err != nil {
		t.Fatalf("UpsertCompetition: %v", err)
	}

	s := tuitest.Settle(t, league.New(newDeps(repos), "eng-premier-league"))
	out := s.View()

	if !strings.Contains(out, "No seasons cached") {
		t.Errorf("expected an empty state\n---\n%s", out)
	}
	if !strings.Contains(out, "footy refresh") {
		t.Errorf("empty state must tell the user the command that fixes it\n---\n%s", out)
	}
}

func TestUnknownCompetitionSurfacesAnError(t *testing.T) {
	repos := storagetest.New(t)
	s := tuitest.Settle(t, league.New(newDeps(repos), "no-such-league"))

	if out := s.View(); !strings.Contains(out, "Could not load standings") {
		t.Errorf("expected an error state\n---\n%s", out)
	}
}

func TestCursorMovesAndClampsAtBothEnds(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())

	// Up from the first row must stay on the first row, not wrap or panic.
	s = tuitest.Key(t, s, "up")
	if got := selectedName(t, s); got != "Arsenal" {
		t.Errorf("cursor at top moved to %q, want it to stay on Arsenal", got)
	}

	s = tuitest.Key(t, s, "down")
	if got := selectedName(t, s); got != "Manchester City" {
		t.Errorf("after one down, selection = %q", got)
	}

	for i := 0; i < 10; i++ {
		s = tuitest.Key(t, s, "down")
	}
	if got := selectedName(t, s); got != "Luton Town" {
		t.Errorf("cursor past the end = %q, want it clamped to Luton Town", got)
	}
}

func TestOnlyTwoMostRecentSeasonsAreOffered(t *testing.T) {
	repos := storagetest.New(t)
	comp := storagetest.PremierLeague()
	for _, label := range []string{"2023", "2024", "2025"} {
		storagetest.SeedCompetition(t, repos, comp, label, storagetest.SampleStandings())
	}

	s := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), comp.ID)), 100, 30)
	out := s.View()

	if !strings.Contains(out, "2025-26") || !strings.Contains(out, "2024-25") {
		t.Errorf("expected the two most recent seasons in the season bar\n---\n%s", out)
	}
	if strings.Contains(out, "2023-24") {
		t.Errorf("League screen is capped at two seasons but offered a third\n---\n%s", out)
	}
}

func TestSeasonSwitchLoadsThatSeasonsTable(t *testing.T) {
	repos := storagetest.New(t)
	comp := storagetest.PremierLeague()
	storagetest.SeedCompetition(t, repos, comp, "2025", storagetest.SampleStandings())
	storagetest.SeedCompetition(t, repos, comp, "2024", []storagetest.TeamRow{
		{ID: "eng-leicester-city", Name: "Leicester City", Position: 1, MP: 38, W: 30, D: 5, L: 3, GF: 80, GA: 30, Pts: 95, XG: 70.1, XGA: 35.5},
	})

	s := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), comp.ID)), 100, 30)
	if out := s.View(); !strings.Contains(out, "Arsenal") {
		t.Fatalf("expected to start on 2025-26\n---\n%s", out)
	}

	// "left" moves to the older season.
	s = tuitest.Key(t, s, "left")
	out := s.View()
	if !strings.Contains(out, "Leicester City") {
		t.Errorf("after switching season, expected the 2024-25 table\n---\n%s", out)
	}
	if strings.Contains(out, "Arsenal") {
		t.Errorf("previous season's rows leaked into the new season's table\n---\n%s", out)
	}
}

func TestEnterWithoutATeamRouteShowsStatusInsteadOfCrashing(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())

	_, cmd := s.Update(enterKey())
	msg := tuitest.CapturedCmd(cmd)
	status, ok := msg.(core.StatusMsg)
	if !ok {
		t.Fatalf("expected a StatusMsg when no Team route is wired, got %#v", msg)
	}
	if status.Text == "" {
		t.Error("status message should explain why nothing happened")
	}
}

func TestEnterPushesTheTeamScreenForTheSelectedRow(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())

	var gotTeam models.Team
	var gotComp, gotSeason string
	deps := newDeps(repos)
	deps.Nav.Team = func(_ core.Deps, team models.Team, competitionID, seasonID string) core.Screen {
		gotTeam, gotComp, gotSeason = team, competitionID, seasonID
		return stubScreen{}
	}

	s := tuitest.Resize(t, tuitest.Settle(t, league.New(deps, "eng-premier-league")), 100, 30)
	s = tuitest.Key(t, s, "down") // select Manchester City

	_, cmd := s.Update(enterKey())
	msg := tuitest.CapturedCmd(cmd)
	if _, ok := msg.(core.PushMsg); !ok {
		t.Fatalf("expected a PushMsg, got %#v", msg)
	}
	if gotTeam.Name != "Manchester City" {
		t.Errorf("drilled into %q, want Manchester City", gotTeam.Name)
	}
	if gotComp != "eng-premier-league" {
		t.Errorf("competition = %q", gotComp)
	}
	if gotSeason != "eng-premier-league:2025" {
		t.Errorf("season = %q, want the season currently on screen", gotSeason)
	}
}

// selectedName reads back which team the cursor is on by finding the row
// the table marked as selected.
func selectedName(t *testing.T, s core.Screen) string {
	t.Helper()
	m, ok := s.(*league.Model)
	if !ok {
		t.Fatalf("expected *league.Model, got %T", s)
	}
	row, ok := m.SelectedRow()
	if !ok {
		t.Fatal("no row selected")
	}
	return row.Team.Name
}

func TestFreshnessShowsWhenTheSeasonWasLastRefreshed(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())

	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{
		Provider: "understat", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		StartedAt: time.Now().Add(-3 * time.Hour), FinishedAt: time.Now().Add(-3 * time.Hour),
		Status: models.SyncStatusOK, RowsUpserted: 20,
	}); err != nil {
		t.Fatalf("RecordSyncRun: %v", err)
	}

	s := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), "eng-premier-league")), 100, 30)
	if out := s.View(); !strings.Contains(out, "updated 3 hours ago") {
		t.Errorf("expected a freshness indicator\n---\n%s", out)
	}
}

// "We tried an hour ago and it broke" is what the user needs to act on, so
// a failed attempt must not be reported as though the data is simply old.
func TestAFailedRefreshIsReportedAsAFailureNotAsStaleness(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())

	if err := repos.RecordSyncRun(context.Background(), models.SyncRun{
		Provider: "understat", CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		StartedAt: time.Now().Add(-time.Hour), FinishedAt: time.Now().Add(-time.Hour),
		Status: models.SyncStatusFailed, Error: "upstream unreachable",
	}); err != nil {
		t.Fatalf("RecordSyncRun: %v", err)
	}

	out := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), "eng-premier-league")), 100, 30).View()
	if !strings.Contains(out, "refresh failed 1 hour ago") {
		t.Errorf("expected the failure surfaced\n---\n%s", out)
	}
	if strings.Contains(out, "updated 1 hour ago") {
		t.Errorf("a failed attempt was reported as a successful update\n---\n%s", out)
	}
}

// Data that predates sync logging, or was seeded by hand, has no run to
// report — that's normal and must not produce a misleading note.
func TestNoSyncHistoryShowsNoFreshnessNote(t *testing.T) {
	s, _ := screen(t, storagetest.SampleStandings())

	out := s.View()
	if strings.Contains(out, "updated") || strings.Contains(out, "refresh failed") {
		t.Errorf("expected no freshness note without a sync log entry\n---\n%s", out)
	}
}

// A narrow terminal has to give something up. Squeezing the team column
// would turn "Manchester United" and "Manchester City" into two identical
// truncated rows, making the table actively wrong rather than merely less
// detailed — so the derived columns go instead.
func TestNarrowTerminalsDropStatColumnsNotTeamNames(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", []storagetest.TeamRow{
		{ID: "eng-manchester-united", Name: "Manchester United", Position: 1, MP: 38, Pts: 85, GF: 71, GA: 27, XG: 77.5, XGA: 33.1},
		{ID: "eng-manchester-city", Name: "Manchester City", Position: 2, MP: 38, Pts: 78, GF: 77, GA: 35, XG: 79.4, XGA: 46.5},
		{ID: "eng-wolves", Name: "Wolverhampton Wanderers", Position: 3, MP: 38, Pts: 20, GF: 27, GA: 68, XG: 38.4, XGA: 68.3},
	})

	for _, width := range []int{70, 80, 95, 120} {
		s := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), "eng-premier-league")), width, 24)
		out := s.View()

		for _, name := range []string{"Manchester United", "Manchester City", "Wolverhampton Wanderers"} {
			if !strings.Contains(out, name) {
				t.Errorf("width %d truncated %q — the two Manchester clubs must stay distinguishable\n---\n%s",
					width, name, out)
			}
		}
		// The core table is never sacrificed.
		for _, col := range []string{"MP", "GD", "Pts"} {
			if !strings.Contains(out, col) {
				t.Errorf("width %d dropped the core column %q\n---\n%s", width, col, out)
			}
		}
	}
}

func TestWideTerminalsShowTheExpectedGoalsColumns(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())

	out := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), "eng-premier-league")), 120, 24).View()
	for _, col := range []string{"xG", "xGA", "xGD"} {
		if !strings.Contains(out, col) {
			t.Errorf("a wide terminal should show %q\n---\n%s", col, out)
		}
	}
}

// xG and xGA are a pair: attacking output means little without what the
// team conceded, so the step below both is neither.
func TestExpectedGoalsColumnsAreDroppedAsAPair(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025", storagetest.SampleStandings())

	for _, width := range []int{60, 70, 78, 84, 90, 100, 120} {
		out := tuitest.Resize(t, tuitest.Settle(t, league.New(newDeps(repos), "eng-premier-league")), width, 24).View()
		hasXG := strings.Contains(out, " xG ") || strings.Contains(out, "  xG")
		hasXGA := strings.Contains(out, "xGA")
		if hasXG && !hasXGA {
			t.Errorf("width %d shows xG without xGA\n---\n%s", width, out)
		}
	}
}
