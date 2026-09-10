package trophies_test

import (
	"context"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/trophies"
)

func TestSeedFileParsesAndIsWellFormed(t *testing.T) {
	entries, err := trophies.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("seed file is empty")
	}

	validTypes := map[string]bool{
		"league_title": true, "domestic_cup": true, "continental_cup": true, "other": true,
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.ID == "" {
			t.Errorf("entry with no id: %+v", e)
		}
		for _, id := range append([]string{e.ID}, e.Aliases...) {
			if seen[id] {
				t.Errorf("id/alias %q used twice", id)
			}
			seen[id] = true
		}
		if len(e.Honours) == 0 {
			t.Errorf("%s has an entry but no honours", e.ID)
		}
		for _, h := range e.Honours {
			if h.Title == "" {
				t.Errorf("%s has an honour with no title", e.ID)
			}
			// An unknown type would sort into the wrong group and never
			// be caught at runtime.
			if !validTypes[h.Type] {
				t.Errorf("%s: %q has unknown trophy type %q", e.ID, h.Title, h.Type)
			}
			if len(h.Seasons) == 0 {
				t.Errorf("%s: %q lists no seasons — the file only claims honours it can name", e.ID, h.Title)
			}
			for _, s := range h.Seasons {
				if s == "" {
					t.Errorf("%s: %q has an empty season", e.ID, h.Title)
				}
			}
		}
	}
}

func seedTeams(t *testing.T, repos storage.Repositories, ids ...string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		if err := repos.UpsertTeam(ctx, models.Team{ID: id, Name: id, Gender: "men"}); err != nil {
			t.Fatalf("UpsertTeam %s: %v", id, err)
		}
	}
}

func TestSeedAppliesHonoursToCachedTeams(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, "eng-liverpool")

	report, err := trophies.Seed(context.Background(), repos)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if report.Teams != 1 {
		t.Errorf("applied to %d teams, want 1", report.Teams)
	}

	got, err := repos.TeamTrophies(context.Background(), "eng-liverpool")
	if err != nil {
		t.Fatalf("TeamTrophies: %v", err)
	}
	// Six European Cups plus the two Premier League titles inside the
	// file's stated 2000-01 cutoff.
	if len(got) != 8 {
		t.Errorf("got %d honours, want 8: %+v", len(got), got)
	}
	if got[0].SeasonLabel != "2024-25" {
		t.Errorf("honours should come back most recent first, got %q", got[0].SeasonLabel)
	}
}

// Re-seeding corrected data must replace the old rows, not stack a second
// copy on top of them.
func TestSeedIsIdempotent(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, "eng-liverpool")

	var counts []int
	for i := 0; i < 3; i++ {
		if _, err := trophies.Seed(context.Background(), repos); err != nil {
			t.Fatalf("Seed pass %d: %v", i, err)
		}
		got, err := repos.TeamTrophies(context.Background(), "eng-liverpool")
		if err != nil {
			t.Fatalf("TeamTrophies: %v", err)
		}
		counts = append(counts, len(got))
	}
	if counts[0] != counts[1] || counts[1] != counts[2] {
		t.Errorf("honour counts drifted across re-seeds: %v", counts)
	}
}

func TestSeedIgnoresUncachedClubs(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, "eng-liverpool")

	report, err := trophies.Seed(context.Background(), repos)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if report.Unused == 0 {
		t.Error("entries for uncached clubs should be counted as unused")
	}
}

// Deleting by source must not touch rows another source contributed.
func TestReseedingLeavesOtherSourcesAlone(t *testing.T) {
	repos := storagetest.New(t)
	ctx := context.Background()
	seedTeams(t, repos, "eng-liverpool")

	if err := repos.InsertTrophy(ctx, models.Trophy{
		TeamID: "eng-liverpool", Title: "Hand-added Cup", SeasonLabel: "1899-00",
		TrophyType: "domestic_cup", Source: "manual",
	}); err != nil {
		t.Fatalf("InsertTrophy: %v", err)
	}

	if _, err := trophies.Seed(ctx, repos); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	got, err := repos.TeamTrophies(ctx, "eng-liverpool")
	if err != nil {
		t.Fatalf("TeamTrophies: %v", err)
	}
	var found bool
	for _, tr := range got {
		if tr.Source == "manual" {
			found = true
		}
	}
	if !found {
		t.Error("re-seeding deleted a trophy contributed by another source")
	}
}

func TestUncuratedClubGetsNoHonours(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, "eng-brentford")

	if _, err := trophies.Seed(context.Background(), repos); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	got, err := repos.TeamTrophies(context.Background(), "eng-brentford")
	if err != nil {
		t.Fatalf("TeamTrophies: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("uncurated club got %d honours from nowhere", len(got))
	}
}
