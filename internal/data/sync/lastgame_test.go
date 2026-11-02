package sync_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

var lastGameDay = time.Date(2026, 10, 31, 19, 45, 0, 0, time.UTC)

// fakeDetails stands in for ESPN's summaries and day scoreboards.
type fakeDetails struct {
	details map[string]models.MatchDetail // ESPN event ID → detail
	days    map[string][]models.Match     // "2006-01-02" → that day's games
	teams   []models.Team                 // the clubs in them
	asked   []string                      // "detail 4001", "day 2026-10-31"
	err     error                         // returned by everything when set
}

func (f *fakeDetails) MatchDetail(_ context.Context, _ provider.CompetitionRef, id string) (models.MatchDetail, error) {
	f.asked = append(f.asked, "detail "+id)
	if f.err != nil {
		return models.MatchDetail{}, f.err
	}
	d, ok := f.details[id]
	if !ok {
		return models.MatchDetail{}, provider.ErrNotFound
	}
	return d, nil
}

func (f *fakeDetails) DayMatches(_ context.Context, _ provider.CompetitionRef, day time.Time) ([]models.Match, []models.Team, error) {
	key := day.Format("2006-01-02")
	f.asked = append(f.asked, "day "+key)
	if f.err != nil {
		return nil, nil, f.err
	}
	return f.days[key], f.teams, nil
}

func detailFor(final bool) models.MatchDetail {
	return models.MatchDetail{
		Final: final, Venue: "Harrowdene Park",
		Home: models.MatchSide{Formation: "4-3-3", Lineup: []models.LineupPlayer{
			{Name: "Tom Ashdown", PlayerID: "espn:9101", Starter: true, Place: 9, Goals: 1},
			{Name: "Ray Colley", PlayerID: "espn:9105"},
		}},
		Events: []models.MatchEvent{{Kind: models.EventGoal, Side: models.SideHome, Period: 1, Minute: 23,
			Player: "Tom Ashdown", PlayerID: "espn:9101", Other: "Leo Marsh", OtherID: "espn:9102"}},
	}
}

// seedLastGame stores a league game (from understat, so no ESPN ID) and an
// older cup game (from ESPN) between Wessex and Northbridge.
func seedLastGame(t *testing.T) storage.Repositories {
	t.Helper()
	ctx := context.Background()
	repos := storagetest.New(t)
	league := models.Competition{ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men"}
	cup := models.Competition{ID: "eng-fa-cup", Name: "FA Cup", Country: "England", Gender: "men", Format: "cup", Category: models.CategoryCup}
	rows := []storagetest.TeamRow{{ID: "eng-wessex-rovers", Name: "Wessex Rovers"}, {ID: "eng-northbridge", Name: "Northbridge City"}}
	pl := storagetest.SeedCompetition(t, repos, league, "2026", rows)
	fa := storagetest.SeedCompetition(t, repos, cup, "2026", nil)
	must(t, repos.UpsertMatches(ctx, []models.Match{
		{ID: "understat-77", CompetitionID: league.ID, SeasonID: pl, Kickoff: lastGameDay, Status: models.MatchFullTime,
			HomeTeamID: "eng-wessex-rovers", AwayTeamID: "eng-northbridge", HomeScore: new(1), AwayScore: new(1),
			Source: "understat", UpdatedAt: lastGameDay},
		{ID: "espn-4001", CompetitionID: cup.ID, SeasonID: fa, Kickoff: lastGameDay.AddDate(0, 0, -10), Status: models.MatchFullTime,
			HomeTeamID: "eng-northbridge", AwayTeamID: "eng-wessex-rovers", HomeScore: new(0), AwayScore: new(2),
			Source: "espn", UpdatedAt: lastGameDay},
	}))
	must(t, repos.PutExternalRef(ctx, "match", "espn-4001", "espn", "4001"))
	must(t, repos.PutExternalRef(ctx, "player", "p-ashdown", "espn", "9101"))
	return repos
}

func TestLastGameFromAnotherSourceIsFoundOnItsDay(t *testing.T) {
	ctx := context.Background()
	repos := seedLastGame(t)
	src := &fakeDetails{
		details: map[string]models.MatchDetail{"5001": detailFor(true)},
		days: map[string][]models.Match{"2026-10-31": {
			{ID: "espn:5000", Kickoff: lastGameDay, HomeTeamID: "espn:7300", AwayTeamID: "espn:7301"},
			{ID: "espn:5001", Kickoff: lastGameDay, HomeTeamID: "espn:7101", AwayTeamID: "espn:7102"},
		}},
		teams: []models.Team{{ID: "espn:7300", Name: "Fenmoor Athletic"}, {ID: "espn:7301", Name: "Lowmarsh"},
			{ID: "espn:7101", Name: "Wessex Rovers FC"}, {ID: "espn:7102", Name: "Northbridge City"}},
	}

	if m, due := sync.LastGameDue(ctx, repos, "eng-wessex-rovers"); !due || m.ID != "understat-77" {
		t.Fatalf("due %v for %s; the newest game has no detail", due, m.ID)
	}
	res, err := sync.RefreshLastGame(ctx, repos, src, "eng-wessex-rovers", lastGameDay)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Fetched || res.Requests != 2 || len(src.asked) != 2 || src.asked[0] != "day 2026-10-31" || src.asked[1] != "detail 5001" {
		t.Errorf("result %+v, asked %v; want that day's scoreboard, then the game", res, src.asked)
	}
	if id, err := repos.ProviderID(ctx, "match", "understat-77", "espn"); err != nil || id != "5001" {
		t.Errorf("the ESPN ID should be recorded: %q, %v", id, err)
	}
	if id, err := repos.ExternalRef(ctx, "team", "espn", "7101"); err != nil || id != "eng-wessex-rovers" {
		t.Errorf("the clubs' ESPN IDs should be recorded too: %q, %v", id, err)
	}

	d, err := repos.MatchDetail(ctx, "understat-77")
	if err != nil {
		t.Fatal(err)
	}
	if d.Venue != "Harrowdene Park" || len(d.Events) != 1 {
		t.Errorf("stored %+v", d)
	}
	if e := d.Events[0]; e.PlayerID != "p-ashdown" || e.OtherID != "" || e.Other != "Leo Marsh" {
		t.Errorf("player IDs: a known player is canonical, an unknown one dropped: %+v", e)
	}
	if p := d.Home.Lineup; p[0].PlayerID != "p-ashdown" || p[1].PlayerID != "" {
		t.Errorf("lineup IDs %+v", p)
	}

	src.asked = nil
	if res, err := sync.RefreshLastGame(ctx, repos, src, "eng-northbridge", lastGameDay); err != nil || res.Fetched || len(src.asked) != 0 {
		t.Errorf("the other club shares the game: %+v, %v, asked %v", res, err, src.asked)
	}
	if _, due := sync.LastGameDue(ctx, repos, "eng-wessex-rovers"); due {
		t.Error("a final detail is kept for good")
	}
}

func TestLastGameWithAnESPNIDIsOneRequest(t *testing.T) {
	ctx := context.Background()
	repos := seedLastGame(t)
	// Only the cup game: drop the league one by making the cup game newer.
	must(t, repos.UpsertMatches(ctx, []models.Match{{ID: "espn-4002", CompetitionID: "eng-fa-cup", SeasonID: "eng-fa-cup:2026",
		Kickoff: lastGameDay.Add(48 * time.Hour), Status: models.MatchFullTime,
		HomeTeamID: "eng-wessex-rovers", AwayTeamID: "eng-northbridge", HomeScore: new(3), AwayScore: new(0),
		Source: "espn", UpdatedAt: lastGameDay}}))
	must(t, repos.PutExternalRef(ctx, "match", "espn-4002", "espn", "4002"))
	src := &fakeDetails{details: map[string]models.MatchDetail{"4002": detailFor(true)}}

	res, err := sync.RefreshLastGame(ctx, repos, src, "eng-wessex-rovers", lastGameDay)
	if err != nil || !res.Fetched || res.Match.ID != "espn-4002" || len(src.asked) != 1 || src.asked[0] != "detail 4002" {
		t.Errorf("result %+v, %v, asked %v", res, err, src.asked)
	}
}

func TestLastGameFetchedBeforeFullTimeIsFetchedAgain(t *testing.T) {
	ctx := context.Background()
	repos := seedLastGame(t)
	must(t, repos.PutExternalRef(ctx, "match", "understat-77", "espn", "5001"))
	src := &fakeDetails{details: map[string]models.MatchDetail{"5001": detailFor(false)}}

	if _, err := sync.RefreshLastGame(ctx, repos, src, "eng-wessex-rovers", lastGameDay); err != nil {
		t.Fatal(err)
	}
	if _, due := sync.LastGameDue(ctx, repos, "eng-wessex-rovers"); !due {
		t.Error("a detail from before full time is still due")
	}
	src.details["5001"] = detailFor(true)
	if res, err := sync.RefreshLastGame(ctx, repos, src, "eng-wessex-rovers", lastGameDay); err != nil || !res.Fetched {
		t.Errorf("refetch %+v, %v", res, err)
	}
	if d, _ := repos.MatchDetail(ctx, "understat-77"); !d.Final || len(d.Events) != 1 {
		t.Errorf("replaced whole, not added to: %+v", d)
	}
}

func TestALastGameTheSourceLacksIsNotAskedAgain(t *testing.T) {
	ctx := context.Background()
	repos := seedLastGame(t)
	src := &fakeDetails{days: map[string][]models.Match{}}

	res, err := sync.RefreshLastGame(ctx, repos, src, "eng-wessex-rovers", lastGameDay)
	if err != nil || !res.Unavailable || res.Fetched || res.Requests != 3 {
		t.Errorf("result %+v, %v; want three days tried, then marked unavailable", res, err)
	}
	if _, due := sync.LastGameDue(ctx, repos, "eng-wessex-rovers"); due {
		t.Error("an unavailable game must not be asked for again")
	}
	if d, err := repos.MatchDetail(ctx, "understat-77"); err != nil || len(d.Events) != 0 || len(d.Home.Lineup) != 0 {
		t.Errorf("the marker is an empty detail: %+v, %v", d, err)
	}
}

func TestALastGameRefreshFailureLeavesItDue(t *testing.T) {
	ctx := context.Background()
	repos := seedLastGame(t)
	src := &fakeDetails{err: provider.ErrRateLimited}

	if _, err := sync.RefreshLastGame(ctx, repos, src, "eng-wessex-rovers", lastGameDay); !errors.Is(err, provider.ErrRateLimited) {
		t.Errorf("got %v", err)
	}
	if _, due := sync.LastGameDue(ctx, repos, "eng-wessex-rovers"); !due {
		t.Error("a failed fetch must leave the game due")
	}
}

func TestAClubWithNoGamesHasNoLastGame(t *testing.T) {
	repos := storagetest.New(t)
	src := &fakeDetails{}
	if res, err := sync.RefreshLastGame(context.Background(), repos, src, "eng-nobody", lastGameDay); err != nil || res.Fetched || len(src.asked) != 0 {
		t.Errorf("%+v, %v, asked %v", res, err, src.asked)
	}
}
