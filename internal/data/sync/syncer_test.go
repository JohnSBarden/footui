package sync_test

import (
	"context"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	"github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/storage/sqlite"
)

// fakeSource is a hand-written stand-in for a real provider (per the
// design doc's testing approach: interfaces here are small enough that a
// mocking framework isn't needed), so SyncCompetitionSeason can be tested
// without a network call.
type fakeSource struct{}

func (fakeSource) Standings(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	xg, xga := 40.0, 20.0
	pos := 1
	return []models.TeamSeasonStats{{
		TeamID: "eng-arsenal", CompetitionID: comp.CompetitionID, SeasonID: season.SeasonID,
		Position: &pos, MatchesPlayed: 10, Wins: 8, Draws: 1, Losses: 1,
		GoalsFor: 25, GoalsAgainst: 10, GoalDiff: 15, Points: 25,
		XG: &xg, XGA: &xga, Source: "fake", UpdatedAt: time.Now(),
	}}, nil
}

func (fakeSource) Teams(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	return []models.Team{{ID: "eng-arsenal", Name: "Arsenal", Country: "England", Gender: "men"}}, nil
}

func (fakeSource) Players(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Player, error) {
	return []models.Player{{ID: "fake-1", Name: "Test Player", Position: "F", Gender: "men"}}, nil
}

func (fakeSource) AllRosterStats(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	xg, xa := 5.0, 2.0
	return []models.PlayerSeasonStats{{
		PlayerID: "fake-1", TeamID: "eng-arsenal", CompetitionID: comp.CompetitionID, SeasonID: season.SeasonID,
		Appearances: 10, Minutes: 900, Goals: 6, Assists: 2, XG: &xg, XA: &xa,
		Source: "fake", UpdatedAt: time.Now(),
	}}, nil
}

func TestSyncCompetitionSeasonPersistsThroughRealStore(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	syncer := sync.New(fakeSource{}, store)

	comp := models.Competition{ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men"}
	season := sync.NewSeason(comp.ID, "2025")

	res, err := syncer.SyncCompetitionSeason(context.Background(), comp, season)
	if err != nil {
		t.Fatalf("SyncCompetitionSeason: %v", err)
	}
	if res.Teams != 1 || res.TeamStats != 1 || res.Players != 1 || res.PlayerStats != 1 {
		t.Fatalf("unexpected result counts: %+v", res)
	}

	standings, err := store.Standings(context.Background(), comp.ID, season.ID)
	if err != nil {
		t.Fatalf("Standings: %v", err)
	}
	if len(standings) != 1 || standings[0].TeamID != "eng-arsenal" || standings[0].Points != 25 {
		t.Fatalf("standings not persisted correctly: %+v", standings)
	}

	roster, err := store.RosterForTeamSeason(context.Background(), "eng-arsenal", comp.ID, season.ID)
	if err != nil {
		t.Fatalf("RosterForTeamSeason: %v", err)
	}
	if len(roster) != 1 || roster[0].Goals != 6 {
		t.Fatalf("roster not persisted correctly: %+v", roster)
	}
}
