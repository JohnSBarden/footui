package sync

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

// fakeDetail is a DetailSource answering from maps and counting calls.
type fakeDetail struct {
	squads  map[string][]provider.SquadMember // espn team ID → roster
	calls   []string                          // "comp/year/athlete"
	keepers []bool
	err     error
	noSlug  map[string]bool
}

func (f *fakeDetail) HasDetail(id string) bool { return !f.noSlug[id] }

func (f *fakeDetail) AthleteSeason(_ context.Context, comp string, year int, athlete string, keeper bool) (models.PlayerSeasonDetail, error) {
	if f.err != nil {
		return models.PlayerSeasonDetail{}, f.err
	}
	f.calls = append(f.calls, comp+"/"+itoa(year)+"/"+athlete)
	f.keepers = append(f.keepers, keeper)
	cs := 10
	return models.PlayerSeasonDetail{CleanSheets: &cs}, nil
}

func (f *fakeDetail) Squad(_ context.Context, _ string, _ int, team string) ([]provider.SquadMember, error) {
	return f.squads[team], nil
}

func itoa(n int) string { return strconv.Itoa(n) }

// October 2026: the Premier League's current season is 2026-27.
var detailNow = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

// seedDetailPlayer caches a player with lines in two Premier League
// seasons for Arsenal, and Arsenal's ESPN team ID.
func seedDetailPlayer(t *testing.T, name, position string) storage.Repositories {
	t.Helper()
	repos := storagetest.New(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repos.UpsertCompetition(ctx, models.Competition{ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men"}))
	must(repos.UpsertTeam(ctx, models.Team{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}))
	must(repos.PutExternalRef(ctx, "team", "eng-arsenal", "espn", "359"))
	must(repos.UpsertPlayer(ctx, models.Player{ID: "understat-1", Name: name, Position: position}))
	for _, season := range []string{"eng-premier-league:2026", "eng-premier-league:2024"} {
		must(repos.UpsertSeason(ctx, models.Season{ID: season, CompetitionID: "eng-premier-league", Label: season[len(season)-4:]}))
		must(repos.UpsertPlayerSeasonStats(ctx, models.PlayerSeasonStats{
			PlayerID: "understat-1", TeamID: "eng-arsenal", CompetitionID: "eng-premier-league",
			SeasonID: season, Minutes: 900, Source: "understat",
		}))
	}
	return repos
}

func TestDetailFetchesEveryLine(t *testing.T) {
	repos := seedDetailPlayer(t, "David Raya", "GK")
	ctx := context.Background()
	if err := repos.PutExternalRef(ctx, "player", "understat-1", "espn", "196176"); err != nil {
		t.Fatal(err)
	}
	src := &fakeDetail{}
	res, err := RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 2 || len(src.calls) != 2 {
		t.Fatalf("fetched %d, calls %v", res.Fetched, src.calls)
	}
	if !src.keepers[0] {
		t.Error("a GK must be fetched as a keeper")
	}
	rows, _ := repos.PlayerSeasonDetails(ctx, "understat-1")
	if len(rows) != 2 || !rows[0].FetchedAt.Equal(detailNow) || rows[0].TeamID != "eng-arsenal" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestDetailPastSeasonFetchedOnce(t *testing.T) {
	repos := seedDetailPlayer(t, "David Raya", "GK")
	ctx := context.Background()
	_ = repos.PutExternalRef(ctx, "player", "understat-1", "espn", "196176")
	src := &fakeDetail{}
	if _, err := RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow, time.Hour); err != nil {
		t.Fatal(err)
	}
	src.calls = nil
	res, err := RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow.Add(2*time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Two hours on, the current season is stale; 2024-25 never is.
	if len(src.calls) != 1 || src.calls[0] != "eng-premier-league/2026/196176" || res.Fresh != 1 {
		t.Errorf("calls %v, fresh %d", src.calls, res.Fresh)
	}
}

func TestDetailCurrentSeasonHonoursMaxAge(t *testing.T) {
	repos := seedDetailPlayer(t, "David Raya", "GK")
	ctx := context.Background()
	_ = repos.PutExternalRef(ctx, "player", "understat-1", "espn", "196176")
	src := &fakeDetail{}
	_, _ = RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow, time.Hour)
	src.calls = nil
	res, _ := RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow.Add(30*time.Minute), time.Hour)
	if len(src.calls) != 0 || res.Fresh != 2 {
		t.Errorf("within maxAge nothing is fetched: calls %v fresh %d", src.calls, res.Fresh)
	}
	// maxAge 0 is a refresh the user asked for: the current season again.
	_, _ = RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow.Add(30*time.Minute), 0)
	if len(src.calls) != 1 {
		t.Errorf("maxAge 0 refetches the current season: calls %v", src.calls)
	}
}

func TestDetailResolvesThroughSquad(t *testing.T) {
	repos := seedDetailPlayer(t, "Gabriel", "D")
	ctx := context.Background()
	src := &fakeDetail{squads: map[string][]provider.SquadMember{"359": {
		{ID: "277385", Name: "William Saliba", Position: "D"},
		{ID: "236322", Name: "Gabriel Magalhães", Position: "D"},
	}}}
	res, err := RefreshPlayerDetail(ctx, repos, src, "understat-1", detailNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if res.Fetched != 2 || src.calls[0] != "eng-premier-league/2026/236322" || src.keepers[0] {
		t.Fatalf("res %+v calls %v keepers %v", res, src.calls, src.keepers)
	}
	if id, err := repos.ProviderID(ctx, "player", "understat-1", "espn"); err != nil || id != "236322" {
		t.Errorf("the match must be recorded: %q %v", id, err)
	}
	rows, _ := repos.PlayerSeasonDetails(ctx, "understat-1")
	if rows[0].Position != "D" {
		t.Errorf("position from the squad = %q", rows[0].Position)
	}
}

func TestDetailUnmatchedIsNotAnError(t *testing.T) {
	repos := seedDetailPlayer(t, "Nobody Known", "M")
	src := &fakeDetail{squads: map[string][]provider.SquadMember{"359": {{ID: "1", Name: "William Saliba"}}}}
	res, err := RefreshPlayerDetail(context.Background(), repos, src, "understat-1", detailNow, time.Hour)
	if err != nil || res.Unmatched != 2 || res.Fetched != 0 {
		t.Errorf("res %+v err %v", res, err)
	}
}

func TestDetailSkipsUncoveredCompetitions(t *testing.T) {
	repos := seedDetailPlayer(t, "David Raya", "GK")
	_ = repos.PutExternalRef(context.Background(), "player", "understat-1", "espn", "196176")
	src := &fakeDetail{noSlug: map[string]bool{"eng-premier-league": true}}
	res, err := RefreshPlayerDetail(context.Background(), repos, src, "understat-1", detailNow, time.Hour)
	if err != nil || len(src.calls) != 0 || res.Fetched != 0 {
		t.Errorf("res %+v calls %v err %v", res, src.calls, err)
	}
}

func TestDetailStopsWhenRateLimited(t *testing.T) {
	repos := seedDetailPlayer(t, "David Raya", "GK")
	_ = repos.PutExternalRef(context.Background(), "player", "understat-1", "espn", "196176")
	src := &fakeDetail{err: provider.ErrRateLimited}
	_, err := RefreshPlayerDetail(context.Background(), repos, src, "understat-1", detailNow, time.Hour)
	if !errors.Is(err, provider.ErrRateLimited) {
		t.Errorf("err = %v, want rate limited", err)
	}
}
