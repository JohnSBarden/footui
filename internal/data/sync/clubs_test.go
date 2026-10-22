package sync_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
)

var oct2026 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

// fakeClubs stands in for ESPN's club scoreboards: each competition-year
// returns the matches set for it, between clubs named in names.
type fakeClubs struct {
	*fakeIntl
	names map[string]string // ESPN ID → club name
}

func newFakeClubs() *fakeClubs {
	return &fakeClubs{fakeIntl: newFakeIntl(), names: map[string]string{}}
}

func (f *fakeClubs) CompetitionMatches(_ context.Context, comp provider.CompetitionRef, year int) ([]models.Match, []models.Team, error) {
	key := fmt.Sprintf("%s/%d", comp.CompetitionID, year)
	f.calls = append(f.calls, "matches "+key)
	ms := append([]models.Match(nil), f.matches[key]...)
	c, _ := sync.FindCompetition(comp.CompetitionID)
	seen := map[string]bool{}
	var teams []models.Team
	for _, m := range ms {
		for _, id := range []string{m.HomeTeamID, m.AwayTeamID} {
			if !seen[id] {
				seen[id] = true
				t := models.Team{ID: id, Name: f.names[strings.TrimPrefix(id, "espn:")], Gender: "men"}
				if c.Country != "Europe" {
					t.Country = c.Country
				}
				teams = append(teams, t)
			}
		}
	}
	return ms, teams, nil
}

// seedClub caches a club with a league line in each season given, as a
// league refresh would have left it — with no matches.
func seedClub(t *testing.T, repos storage.Repositories, id, name, country, comp string, seasons ...string) {
	t.Helper()
	ctx := context.Background()
	c, ok := sync.FindCompetition(comp)
	if !ok {
		t.Fatalf("no competition %s", comp)
	}
	mustNoErr(t, repos.UpsertCompetition(ctx, c))
	mustNoErr(t, repos.UpsertTeam(ctx, models.Team{ID: id, Name: name, Country: country, Gender: "men"}))
	for _, s := range seasons {
		se := sync.NewSeason(comp, s)
		mustNoErr(t, repos.UpsertSeason(ctx, se))
		mustNoErr(t, repos.LinkTeamCompetitionSeason(ctx, id, comp, se.ID))
		mustNoErr(t, repos.UpsertTeamSeasonStats(ctx, models.TeamSeasonStats{TeamID: id, CompetitionID: comp,
			SeasonID: se.ID, MatchesPlayed: 8, Source: "fake", UpdatedAt: oct2026}))
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func unitNames(units []sync.Unit) []string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = u.String()
	}
	slices.Sort(out)
	return out
}

// A Premier League club: understat league seasons plus ESPN cup and
// European units.
func TestAClubsPlanIsItsLeagueEuropeAndItsCountrysCups(t *testing.T) {
	now := oct2026
	_, repos := newMatchSyncer(t, newFakeClubs(), &now)
	seedClub(t, repos, "eng-arsenal", "Arsenal", "England", "eng-premier-league", "2024", "2025", "2026")

	team, _ := repos.GetTeam(context.Background(), "eng-arsenal")
	plan, err := sync.PlanClub(context.Background(), repos, team, now, 0)
	mustNoErr(t, err)

	want := []string{
		"eng-fa-cup 2025", "eng-fa-cup 2026", "eng-league-cup 2025", "eng-league-cup 2026",
		"eur-champions-league 2025", "eur-champions-league 2026", "eur-conference-league 2025",
		"eur-conference-league 2026", "eur-europa-league 2025", "eur-europa-league 2026",
	}
	if got := unitNames(plan.Units); !slices.Equal(got, want) {
		t.Errorf("units\n got %v\nwant %v", got, want)
	}
	// 2024-25 reaches into last year; 2023-24 doesn't.
	if len(plan.Leagues) != 1 || plan.Leagues[0].Competition.ID != "eng-premier-league" ||
		!slices.Equal(plan.Leagues[0].Seasons, []string{"2026", "2025", "2024"}) {
		t.Errorf("league seasons %+v", plan.Leagues)
	}
}

// An ESPN league is a unit like a cup; a US club has no European football.
func TestAnESPNLeaguesClubPlansItsLeagueAsUnits(t *testing.T) {
	now := oct2026
	_, repos := newMatchSyncer(t, newFakeClubs(), &now)
	seedClub(t, repos, "usa-austin-fc", "Austin FC", "USA", "usa-mls", "2025", "2026")

	team, _ := repos.GetTeam(context.Background(), "usa-austin-fc")
	plan, err := sync.PlanClub(context.Background(), repos, team, now, 0)
	mustNoErr(t, err)
	want := []string{"usa-mls 2025", "usa-mls 2026", "usa-open-cup 2025", "usa-open-cup 2026"}
	if got := unitNames(plan.Units); !slices.Equal(got, want) || len(plan.Leagues) != 0 {
		t.Errorf("units %v, leagues %+v", got, plan.Leagues)
	}
}

// An understat season with its matches stored is only due again when
// it's the current one and stale.
func TestAnUnderstatSeasonWithMatchesIsDueOnlyWhenCurrentAndStale(t *testing.T) {
	now := oct2026
	_, repos := newMatchSyncer(t, newFakeClubs(), &now)
	ctx := context.Background()
	seedClub(t, repos, "eng-arsenal", "Arsenal", "England", "eng-premier-league", "2025", "2026")
	mustNoErr(t, repos.UpsertTeam(ctx, models.Team{ID: "eng-chelsea", Name: "Chelsea", Country: "England", Gender: "men"}))
	for _, s := range []string{"2025", "2026"} {
		m := fixture("eng-premier-league", 0, now.Add(-72*time.Hour), "x", "y", 1, 0)
		m.SeasonID, m.HomeTeamID, m.AwayTeamID = "eng-premier-league:"+s, "eng-arsenal", "eng-chelsea"
		mustNoErr(t, repos.UpsertMatches(ctx, []models.Match{m}))
		mustNoErr(t, repos.RecordSyncRun(ctx, models.SyncRun{Provider: "understat", CompetitionID: "eng-premier-league",
			SeasonID: m.SeasonID, StartedAt: now.Add(-time.Hour), FinishedAt: now.Add(-time.Hour), Status: models.SyncStatusOK}))
	}
	team, _ := repos.GetTeam(ctx, "eng-arsenal")

	plan, _ := sync.PlanClub(ctx, repos, team, now, 6*time.Hour)
	if len(plan.Leagues) != 0 {
		t.Errorf("fetched an hour ago, within the stale threshold: %+v", plan.Leagues)
	}
	plan, _ = sync.PlanClub(ctx, repos, team, now, 30*time.Minute)
	if len(plan.Leagues) != 1 || !slices.Equal(plan.Leagues[0].Seasons, []string{"2026"}) {
		t.Errorf("only the current season goes stale: %+v", plan.Leagues)
	}
}

// The first English club costs a request per unit; the second, none.
func TestASecondClubOfTheSameCountryCostsNothing(t *testing.T) {
	now := oct2026
	src := newFakeClubs()
	src.names = map[string]string{"11": "Arsenal", "12": "Chelsea", "13": "Wessex Rovers"}
	src.matches["eng-fa-cup/2026"] = []models.Match{fixture("eng-fa-cup", 2025, at(2026, 1, 10, 15), "13", "11", 0, 2)}
	src.matches["eur-champions-league/2026"] = []models.Match{fixture("eur-champions-league", 2026, at(2026, 9, 30, 19), "12", "11", 1, 1)}
	s, repos := newMatchSyncer(t, src, &now)
	seedClub(t, repos, "eng-arsenal", "Arsenal", "England", "eng-premier-league", "2026")
	seedClub(t, repos, "eng-chelsea", "Chelsea", "England", "eng-premier-league", "2026")
	ctx := context.Background()

	for i, id := range []string{"eng-arsenal", "eng-chelsea"} {
		before := src.count("matches")
		team, _ := repos.GetTeam(ctx, id)
		plan, err := sync.PlanClub(ctx, repos, team, now, 0)
		mustNoErr(t, err)
		s.SyncUnits(ctx, plan.Units, nil)
		got := src.count("matches") - before
		if i == 0 && got != 10 || i == 1 && got != 0 {
			t.Errorf("club %d cost %d requests", i+1, got)
		}
	}

	recent, err := repos.RecentMatches(ctx, "eng-arsenal", 5)
	mustNoErr(t, err)
	if len(recent) != 2 || recent[0].CompetitionID != "eur-champions-league" || recent[0].HomeTeamID != "eng-chelsea" ||
		recent[1].AwayTeamID != "eng-arsenal" {
		t.Errorf("recent %+v", recent)
	}
	// A cup tie must not give a club a cup line among its seasons.
	history, _ := repos.TeamHistory(ctx, "eng-arsenal")
	for _, h := range history {
		if h.CompetitionID != "eng-premier-league" {
			t.Errorf("history gained %s", h.CompetitionID)
		}
	}
	// The cup opponent is a club of its own, from England.
	if w, err := repos.GetTeam(ctx, "eng-wessex-rovers"); err != nil || w.Country != "England" || w.National {
		t.Errorf("cup opponent %+v, %v", w, err)
	}
}

// In a cup, a name containing a big club's is a different club unless
// only suffixes differ.
func TestCupClubsMatchCachedOnesOnlyBySuffix(t *testing.T) {
	now := oct2026
	src := newFakeClubs()
	src.names = map[string]string{"21": "FC United of Manchester", "22": "Leeds United", "23": "Manchester United"}
	src.matches["eng-fa-cup/2026"] = []models.Match{
		fixture("eng-fa-cup", 2025, at(2026, 1, 10, 15), "21", "22", 0, 1),
		fixture("eng-fa-cup", 2025, at(2026, 2, 7, 15), "23", "22", 2, 1),
	}
	s, repos := newMatchSyncer(t, src, &now)
	ctx := context.Background()
	seedClub(t, repos, "eng-manchester-united", "Manchester United", "England", "eng-premier-league", "2026")
	seedClub(t, repos, "eng-leeds", "Leeds", "England", "eng-premier-league", "2026")

	cup, _ := sync.FindCompetition("eng-fa-cup")
	s.SyncUnits(ctx, []sync.Unit{{Competition: cup, Year: 2026}}, nil)

	if ms := matchesOf(t, repos, "eng-manchester-united"); len(ms) != 1 {
		t.Errorf("Manchester United has %d matches; FC United of Manchester is another club", len(ms))
	}
	if ms := matchesOf(t, repos, "eng-leeds"); len(ms) != 2 {
		t.Errorf("Leeds United is Leeds: %d matches", len(ms))
	}
	if _, err := repos.GetTeam(ctx, "eng-fc-united-of-manchester"); err != nil {
		t.Errorf("FC United of Manchester wasn't kept as its own club: %v", err)
	}
}

// understatLike returns its league's matches with the season, under
// understat's own IDs.
type understatLike struct{ fakeSource }

func (understatLike) Teams(context.Context, provider.CompetitionRef, provider.SeasonRef) ([]models.Team, error) {
	return []models.Team{
		{ID: "understat:eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"},
		{ID: "understat:eng-chelsea", Name: "Chelsea", Country: "England", Gender: "men"},
	}, nil
}

func (understatLike) SeasonMatches(_ context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Match, error) {
	return []models.Match{{ID: "understat:77", CompetitionID: comp.CompetitionID, SeasonID: season.SeasonID,
		Kickoff: at(2026, 9, 20, 14), Status: models.MatchFullTime, HomeTeamID: "understat:eng-chelsea",
		AwayTeamID: "understat:eng-arsenal", HomeScore: new(0), AwayScore: new(1),
		AwayStats: &models.MatchTeamStats{XG: new(1.7)}, Source: "understat", UpdatedAt: oct2026}}, nil
}

func TestALeagueRefreshStoresItsMatches(t *testing.T) {
	now := oct2026
	_, repos := newMatchSyncer(t, newFakeClubs(), &now)
	ctx := context.Background()
	comp, _ := sync.FindCompetition("eng-premier-league")
	season := sync.NewSeason(comp.ID, "2026")

	res, err := sync.New(understatLike{}, repos).SyncCompetitionSeason(ctx, comp, season)
	mustNoErr(t, err)
	if res.Matches != 1 {
		t.Errorf("result %+v", res)
	}
	recent, _ := repos.RecentMatches(ctx, "eng-arsenal", 5)
	if len(recent) != 1 || recent[0].ID != "understat-77" || recent[0].HomeTeamID != "eng-chelsea" ||
		*recent[0].AwayStats.XG != 1.7 {
		t.Errorf("recent %+v", recent)
	}
	// Refreshing again updates the same match.
	_, err = sync.New(understatLike{}, repos).SyncCompetitionSeason(ctx, comp, season)
	mustNoErr(t, err)
	if n, _ := repos.CountSeasonMatches(ctx, comp.ID, season.ID); n != 1 {
		t.Errorf("%d matches after a second refresh", n)
	}
}
