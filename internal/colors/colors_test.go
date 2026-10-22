package colors_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/colors"
	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

func TestSeedFileParsesAndIsWellFormed(t *testing.T) {
	entries, err := colors.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("seed file is empty")
	}

	seen := map[string]string{}
	for _, e := range entries {
		if e.ID == "" || e.Name == "" || e.Primary == "" {
			t.Errorf("entry %+v is missing a required field", e)
		}
		// A duplicate ID or alias would shadow another club's colours.
		for _, id := range append([]string{e.ID}, e.Aliases...) {
			if prev, dup := seen[id]; dup {
				t.Errorf("id/alias %q used by both %q and %q", id, prev, e.Name)
			}
			seen[id] = e.Name
		}
	}
}

// A colour typo must fail here, not silently fall back at render time.
func TestEverySeededColorIsUsable(t *testing.T) {
	entries, err := colors.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	neutral := styles.DefaultPalette()
	for _, e := range entries {
		if got := styles.TeamPalette(e.Primary, e.Secondary); got.Accent == neutral.Accent {
			t.Errorf("%s: primary %q did not produce a brand accent — is it a valid hex color?", e.Name, e.Primary)
		}
	}
}

func seedTeams(t *testing.T, repos storage.Repositories, teams ...models.Team) {
	t.Helper()
	ctx := context.Background()
	for _, tm := range teams {
		if err := repos.UpsertTeam(ctx, tm); err != nil {
			t.Fatalf("UpsertTeam %s: %v", tm.ID, err)
		}
	}
}

func TestSeedAppliesColorsToCachedTeams(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos,
		models.Team{ID: "eng-arsenal", Name: "Arsenal", Gender: "men"},
		models.Team{ID: "eng-chelsea", Name: "Chelsea", Gender: "men"},
	)

	report, err := colors.Seed(context.Background(), repos)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if report.Applied != 2 {
		t.Errorf("applied to %d teams, want 2", report.Applied)
	}

	got, err := repos.GetTeamColors(context.Background(), "eng-arsenal")
	if err != nil {
		t.Fatalf("GetTeamColors: %v", err)
	}
	if !strings.EqualFold(got.PrimaryHex, "#EF0107") {
		t.Errorf("Arsenal primary = %q", got.PrimaryHex)
	}
	if got.Source != colors.SeedSource {
		t.Errorf("source = %q, want provenance recorded", got.Source)
	}
}

// An entry for an uncached club must not write a team_colors row.
func TestSeedIgnoresEntriesForUncachedTeams(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, models.Team{ID: "eng-arsenal", Name: "Arsenal", Gender: "men"})

	report, err := colors.Seed(context.Background(), repos)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if report.Applied != 1 {
		t.Errorf("applied to %d teams, want only the one cached team", report.Applied)
	}
	if report.Unused == 0 {
		t.Error("the many entries for uncached clubs should be reported as unused")
	}
}

// Uncurated clubs must be reported, not hidden.
func TestSeedReportsUncoloredTeams(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos,
		models.Team{ID: "eng-arsenal", Name: "Arsenal", Gender: "men"},
		models.Team{ID: "eng-some-lower-league-club", Name: "Not Curated FC", Gender: "men"},
	)

	report, err := colors.Seed(context.Background(), repos)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if len(report.Uncolored) != 1 || report.Uncolored[0].Name != "Not Curated FC" {
		t.Errorf("uncolored = %+v, want just the uncurated club", report.Uncolored)
	}
}

// A club whose provider spells its name differently should still find its
// colours through an alias.
func TestAliasesResolveAlternativeTeamIDs(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, models.Team{ID: "eng-wolves", Name: "Wolves", Gender: "men"})

	report, err := colors.Seed(context.Background(), repos)
	if err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if report.Applied != 1 {
		t.Fatalf("alias did not resolve: %+v", report)
	}

	got, err := repos.GetTeamColors(context.Background(), "eng-wolves")
	if err != nil {
		t.Fatalf("GetTeamColors: %v", err)
	}
	if !strings.EqualFold(got.PrimaryHex, "#FDB913") {
		t.Errorf("Wolves primary = %q", got.PrimaryHex)
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	repos := storagetest.New(t)
	seedTeams(t, repos, models.Team{ID: "eng-arsenal", Name: "Arsenal", Gender: "men"})

	for i := 0; i < 3; i++ {
		report, err := colors.Seed(context.Background(), repos)
		if err != nil {
			t.Fatalf("Seed pass %d: %v", i, err)
		}
		if report.Applied != 1 {
			t.Errorf("pass %d applied to %d teams, want 1", i, report.Applied)
		}
	}
}
