package browse_test

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/screens/browse"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

// everyKind adds a nation and a second league to the seed, so the query
// "an" matches a club, a nation, a player and a league.
func everyKind(t *testing.T) storage.Repositories {
	t.Helper()
	repos := seeded(t)
	ctx := context.Background()
	if err := repos.UpsertTeam(ctx, models.Team{ID: "nat-oman", Name: "Oman", Country: "Oman", Gender: "men", National: true}); err != nil {
		t.Fatal(err)
	}
	if err := repos.UpsertCompetition(ctx, models.Competition{ID: "den-superliga", Name: "Danish Superliga", Country: "Denmark", Tier: 1, Gender: "men"}); err != nil {
		t.Fatal(err)
	}
	return repos
}

func TestSearchGroupsResultsClubsNationsPlayersLeagues(t *testing.T) {
	view := open(t, newDeps(everyKind(t)), "an").View()
	idx := func(s string) int { return strings.Index(view, s) }
	for _, label := range []string{"CLUBS", "NATIONS", "PLAYERS", "LEAGUES"} {
		if idx(label) < 0 {
			t.Fatalf("missing group %s\n---\n%s", label, view)
		}
	}
	if !(idx("CLUBS") < idx("NATIONS") && idx("NATIONS") < idx("PLAYERS") && idx("PLAYERS") < idx("LEAGUES")) {
		t.Errorf("group order wrong\n---\n%s", view)
	}
	if !strings.Contains(view, "● Manchester") || !strings.Contains(view, "England") {
		t.Errorf("rows should have a dot and their meta\n---\n%s", view)
	}
}

func TestTheSelectedResultIsMarked(t *testing.T) {
	s := tuitest.Key(t, open(t, newDeps(everyKind(t)), "an"), "tab")
	if !strings.Contains(s.View(), "←") {
		t.Errorf("selected row marker missing\n---\n%s", s.View())
	}
}

func TestSearchCursorCrossesGroupsInOrder(t *testing.T) {
	s := tuitest.Key(t, open(t, newDeps(everyKind(t)), "an"), "tab")
	var names []string
	for i := 0; i < 12; i++ {
		r, _ := s.(*browse.Model).SelectedResult()
		if len(names) == 0 || names[len(names)-1] != r.Name() {
			names = append(names, r.Name())
		}
		s = tuitest.Key(t, s, "j")
	}
	got := strings.Join(names, ",")
	if !strings.HasPrefix(got, "Manchester") || !strings.HasSuffix(got, "Danish Superliga") || !strings.Contains(got, "Oman,Erling Haaland") {
		t.Errorf("cursor order %s", got)
	}
}

func TestEmptyQueryIsOneLeaguesGroup(t *testing.T) {
	view := open(t, newDeps(everyKind(t)), "").View()
	if !strings.Contains(view, "LEAGUES") || strings.Contains(view, "CLUBS") {
		t.Errorf("empty query\n---\n%s", view)
	}
}

func TestSearchFitsEightyColumns(t *testing.T) {
	s := tuitest.Resize(t, open(t, newDeps(everyKind(t)), "an"), 80, 24)
	for _, l := range strings.Split(s.View(), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("overflow %d: %q", lipgloss.Width(l), l)
		}
	}
}
