package browse_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
)

// Nations are searchable before any match is fetched.
func TestNationsAreLabelledNationalTeams(t *testing.T) {
	repos := storagetest.New(t)
	for _, n := range []models.Team{
		{ID: "nat-arcadia", Name: "Arcadia", Gender: "men", National: true},
		{ID: "nat-arcadia-women", Name: "Arcadia", Gender: "women", National: true},
	} {
		if err := repos.UpsertTeam(context.Background(), n); err != nil {
			t.Fatal(err)
		}
	}
	out := open(t, newDeps(repos), "arcadia").View()
	if !strings.Contains(out, "NATIONS 2") || !strings.Contains(out, "women") {
		t.Errorf("results:\n%s", out)
	}
}

func TestInternationalCompetitionsShowTheirCategory(t *testing.T) {
	repos := storagetest.New(t)
	storagetest.SeedInternational(t, repos)
	out := open(t, newDeps(repos), "world cup").View()
	if !strings.Contains(out, "International · tournament · men") {
		t.Errorf("results:\n%s", out)
	}
}
