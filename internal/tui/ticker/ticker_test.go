package ticker

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

var ctx = context.Background()

func followArsenal(t *testing.T) (storage.Repositories, string) {
	t.Helper()
	repos := storagetest.New(t)
	season := storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2026", storagetest.SampleStandings())
	if err := repos.AddFavorite(ctx, models.Favorite{Kind: models.FavoriteTeam, ID: "eng-arsenal", AddedAt: storagetest.Fixed}); err != nil {
		t.Fatal(err)
	}
	return repos, season
}

func match(id string, kickoff time.Time, home, away int) models.Match {
	return models.Match{
		ID: id, CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2026",
		Kickoff: kickoff, Status: models.MatchFullTime,
		HomeTeamID: "eng-arsenal", AwayTeamID: "eng-manchester-city",
		HomeScore: new(home), AwayScore: new(away), Source: "test", UpdatedAt: kickoff,
	}
}

func TestARefreshBringsInAResultAndATableMove(t *testing.T) {
	repos, season := followArsenal(t)
	now := time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)
	before := Take(ctx, repos, now)

	if err := repos.UpsertMatches(ctx, []models.Match{match("m1", now.Add(-24*time.Hour), 2, 1)}); err != nil {
		t.Fatal(err)
	}
	// Arsenal were 1st; City go top.
	for id, pos := range map[string]int{"eng-arsenal": 2, "eng-manchester-city": 1} {
		rows, _ := repos.Standings(ctx, "eng-premier-league", season)
		for _, r := range rows {
			if r.TeamID == id {
				r.Position = new(pos)
				if err := repos.UpsertTeamSeasonStats(ctx, r); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	after := Take(ctx, repos, now)

	got := News(ctx, repos, before, after)
	want := []string{
		"FT  Arsenal 2–1 Manchester City · Premier League",
		"↓ Arsenal down to 2nd in Premier League (from 1st)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("news = %q, want %q", got, want)
	}
	// Nothing changed since: no news twice.
	if again := News(ctx, repos, after, Take(ctx, repos, now)); len(again) != 0 {
		t.Errorf("the same result was news again: %q", again)
	}
}

// A whole-history refresh fills in old results; they aren't news.
func TestOldResultsAreNotNews(t *testing.T) {
	repos, _ := followArsenal(t)
	now := time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)
	before := Take(ctx, repos, now)
	if err := repos.UpsertMatches(ctx, []models.Match{match("old", now.AddDate(0, -2, 0), 0, 3)}); err != nil {
		t.Fatal(err)
	}
	if got := News(ctx, repos, before, Take(ctx, repos, now)); len(got) != 0 {
		t.Errorf("news = %q", got)
	}
	// No snapshot before: nothing to compare against.
	if got := News(ctx, repos, Snapshot{}, Take(ctx, repos, now)); got != nil {
		t.Errorf("news without a before = %q", got)
	}
}

func TestResultShowsAShootOut(t *testing.T) {
	m := match("p", time.Time{}, 1, 1)
	m.Status, m.HomePens, m.AwayPens = models.MatchPens, new(4), new(3)
	if got, want := Result(m, "England", "Spain", "Euro"), "Pens  England 1–1 (4–3) Spain · Euro"; got != want {
		t.Errorf("Result = %q, want %q", got, want)
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 101: "101st", 111: "111th"} {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTheFeedCyclesAndGoesStale(t *testing.T) {
	var f Feed
	t0 := time.Date(2026, 10, 20, 20, 0, 0, 0, time.UTC)
	if f.Current() != "" || f.Advance(t0) {
		t.Fatal("an empty feed shows nothing")
	}
	f.Add(t0, "a", "b")
	if got := f.Current(); got != "a  1/2" {
		t.Errorf("first item = %q", got)
	}
	f.Advance(t0.Add(Every))
	if got := f.Current(); got != "b  2/2" {
		t.Errorf("second item = %q", got)
	}
	f.Advance(t0.Add(2 * Every))
	if got := f.Current(); got != "a  1/2" {
		t.Errorf("the feed should wrap round, got %q", got)
	}
	// Newer news goes in front and is shown at once.
	f.Add(t0.Add(Keep), "c")
	if got := f.Current(); got != "c  1/3" {
		t.Errorf("new news = %q", got)
	}
	if !f.Advance(t0.Add(Keep + time.Second)) {
		t.Fatal("c is still fresh")
	}
	if got := f.Current(); got != "c" {
		t.Errorf("stale items should go, leaving %q", got)
	}
}
