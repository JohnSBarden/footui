package sync_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

type fakeCareer []provider.CareerSeason

func (f fakeCareer) AthleteCareer(context.Context, string) ([]provider.CareerSeason, error) {
	return f, nil
}

var october2026 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// Griezmann: seasons before coverage, cups and friendlies are dropped.
func TestPlayerCareerJobs(t *testing.T) {
	ctx := context.Background()
	repos := storagetest.New(t)
	must(t, repos.UpsertPlayer(ctx, models.Player{ID: "espn-140416", Name: "Antoine Griezmann", Gender: "men"}))
	must(t, repos.PutExternalRef(ctx, "player", "espn-140416", sync.ProviderESPN, "140416"))
	src := fakeCareer{
		{CompetitionID: "esp-la-liga-2", Year: 2009},
		{CompetitionID: "eur-champions-league", Year: 2014},
		{CompetitionID: "esp-la-liga", Year: 2014},
		{CompetitionID: "esp-copa-del-rey", Year: 2014},
		{CompetitionID: "int-friendly", Year: 2014},
		{CompetitionID: "esp-la-liga", Year: 2015},
		{CompetitionID: "usa-mls", Year: 2026},
	}
	jobs, err := sync.PlayerCareerJobs(ctx, repos, src, "espn-140416", october2026)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	var order []string
	for _, j := range jobs {
		got[j.Competition.ID] = j.Seasons
		order = append(order, j.Competition.ID)
	}
	want := map[string][]string{
		"esp-la-liga":          {"2015", "2014"},
		"usa-mls":              {"2026"},
		"eur-champions-league": {"2014"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("jobs = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(order, []string{"esp-la-liga", "usa-mls", "eur-champions-league"}) {
		t.Errorf("order = %v", order)
	}
}

func TestPlayerCareerJobsWithoutAnESPNID(t *testing.T) {
	ctx := context.Background()
	repos := storagetest.New(t)
	must(t, repos.UpsertPlayer(ctx, models.Player{ID: "understat-1", Name: "Nobody", Gender: "men"}))
	_, err := sync.PlayerCareerJobs(ctx, repos, fakeCareer{}, "understat-1", october2026)
	if !errors.Is(err, sync.ErrNoCareer) {
		t.Errorf("err = %v, want ErrNoCareer", err)
	}
}

// A club gets every season footui offers of each league it has a table in.
func TestClubHistoryJobs(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedCompetition(t, repos, storagetest.PremierLeague(), "2025",
		[]storagetest.TeamRow{{ID: "eng-arsenal", Name: "Arsenal", Position: 1, MP: 38}})
	jobs, err := sync.ClubHistoryJobs(context.Background(), repos, "eng-arsenal", []string{"world-cup"}, october2026)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Competition.ID != "eng-premier-league" {
		t.Fatalf("jobs = %+v", jobs)
	}
	s := jobs[0].Seasons
	if len(s) != 13 || s[0] != "2026" || s[len(s)-1] != "2014" {
		t.Errorf("seasons = %v, want 2026 back to 2014", s)
	}
}

// KeepFinished leaves alone a season synced after it ended, but not one
// last synced while it was still being played, nor the current season.
func TestKeepFinished(t *testing.T) {
	ctx := context.Background()
	repos := storagetest.New(t)
	src := &flakySource{failOn: map[string]bool{}}
	cov, _ := sync.FindCoverage("esp-la-liga")
	laLiga := cov.Competition
	current := cov.CurrentSeason(time.Now())

	syncer := sync.New(src, repos)
	syncer.SyncJobs(ctx, []sync.Job{{Competition: laLiga, Seasons: []string{current, "2015"}}}, nil)
	// 2014-15 was last fetched in November 2014, mid-season.
	must(t, repos.RecordSyncRun(ctx, models.SyncRun{
		Provider: "fake", CompetitionID: laLiga.ID, SeasonID: "esp-la-liga:2014",
		StartedAt: time.Date(2014, 11, 1, 0, 0, 0, 0, time.UTC), FinishedAt: time.Date(2014, 11, 1, 0, 1, 0, 0, time.UTC),
		Status: models.SyncStatusOK,
	}))

	syncer.KeepFinished = true
	results := syncer.SyncJobs(ctx, []sync.Job{{Competition: laLiga, Seasons: []string{current, "2015", "2014"}}}, nil)
	fresh := map[string]bool{}
	for _, r := range results {
		fresh[r.Season.ID] = r.Fresh
	}
	want := map[string]bool{"esp-la-liga:" + current: false, "esp-la-liga:2015": true, "esp-la-liga:2014": false}
	if !reflect.DeepEqual(fresh, want) {
		t.Errorf("fresh = %v, want %v", fresh, want)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
