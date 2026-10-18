package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

func intp(v int) *int { return &v }

// A second fetch replaces the first; a stat that was never recorded stays
// nil rather than coming back as zero.
func TestPlayerSeasonDetailRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	d := models.PlayerSeasonDetail{
		PlayerID: "understat-9676", TeamID: "eng-arsenal",
		CompetitionID: "eng-premier-league", SeasonID: "eng-premier-league:2025",
		Position: "G", Minutes: intp(3000), CleanSheets: intp(17), GoalsConceded: intp(24),
		Saves: intp(55), ShotsOnTarget: intp(1), FetchedAt: at,
	}
	if err := s.UpsertPlayerSeasonDetail(ctx, d); err != nil {
		t.Fatal(err)
	}
	d.Minutes, d.CleanSheets, d.Saves = intp(3330), intp(19), intp(60)
	d.FetchedAt = at.Add(time.Hour)
	if err := s.UpsertPlayerSeasonDetail(ctx, d); err != nil {
		t.Fatal(err)
	}

	got, err := s.PlayerSeasonDetails(ctx, "understat-9676")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	g := got[0]
	if *g.Minutes != 3330 || *g.CleanSheets != 19 || *g.Saves != 60 || *g.GoalsConceded != 24 {
		t.Errorf("values not replaced: %+v", g)
	}
	if g.ShotsOnTarget == nil || *g.ShotsOnTarget != 1 {
		t.Errorf("shots on target = %v", g.ShotsOnTarget)
	}
	if g.Position != "G" || !g.FetchedAt.Equal(at.Add(time.Hour)) {
		t.Errorf("position %q fetched %v", g.Position, g.FetchedAt)
	}
	if g.Tackles != nil || g.Interceptions != nil || g.Clearances != nil || g.PassPct != nil || g.Appearances != nil {
		t.Errorf("unrecorded stats must stay nil: %+v", g)
	}
}

func TestProviderIDReverseLookup(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.PutExternalRef(ctx, "player", "understat-9676", "espn", "196176"); err != nil {
		t.Fatal(err)
	}
	id, err := s.ProviderID(ctx, "player", "understat-9676", "espn")
	if err != nil || id != "196176" {
		t.Fatalf("ProviderID = %q, %v", id, err)
	}
	if _, err := s.ProviderID(ctx, "player", "understat-1", "espn"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("missing ref: err = %v, want ErrNotFound", err)
	}
}

func TestSeasonPlayerLines(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, p := range []string{"p1", "p2", "p3"} {
		if err := s.UpsertPlayer(ctx, models.Player{ID: p, Name: p}); err != nil {
			t.Fatal(err)
		}
	}
	lines := []models.PlayerSeasonStats{
		{PlayerID: "p1", TeamID: "t1", CompetitionID: "c", SeasonID: "c:2025", Minutes: 900, Source: "test"},
		{PlayerID: "p2", TeamID: "t2", CompetitionID: "c", SeasonID: "c:2025", Minutes: 1800, Source: "test"},
		{PlayerID: "p3", TeamID: "t1", CompetitionID: "c", SeasonID: "c:2024", Minutes: 900, Source: "test"},
	}
	for _, l := range lines {
		if err := s.UpsertPlayerSeasonStats(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.SeasonPlayerLines(ctx, "c", "c:2025")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d lines, want the 2 in c:2025", len(got))
	}
}
