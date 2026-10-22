package browse

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage/storagetest"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/styles"
	"github.com/JohnSBarden/footui/internal/tui/tuitest"
)

type fakePlayers struct {
	found  []models.FoundPlayer
	career []models.CareerSeason
}

func (f fakePlayers) SearchPlayers(context.Context, string) ([]models.FoundPlayer, error) {
	return f.found, nil
}

func (f fakePlayers) PlayerCareer(context.Context, string) ([]models.CareerSeason, error) {
	return f.career, nil
}

var ronaldo = models.FoundPlayer{Provider: "src", ID: "7", Name: "Cristiano Ronaldo", Context: "Saudi Pro League - Al Nassr"}

func foundDeps(t *testing.T, players fakePlayers) core.Deps {
	t.Helper()
	repos := storagetest.New(t)
	pl := storagetest.PremierLeague()
	storagetest.SeedCompetition(t, repos, pl, "2025", nil)
	storagetest.SeedRoster(t, repos, "eng-manchester-city", pl.ID, pl.ID+":2025",
		[]storagetest.PlayerRow{{ID: "p-haaland", Name: "Erling Haaland", Position: "F"}})
	return core.Deps{
		Ctx: context.Background(), Repos: repos, Theme: styles.Plain(), Players: players,
		Catalog: []core.CatalogEntry{
			{Competition: pl, Seasons: []string{"2025", "2024", "2023"}},
			{Competition: models.Competition{ID: "eng-fa-cup", Category: "cup"}, Seasons: []string{"2024"}},
		},
	}
}

// searchedFurther opens Browse on query and lets the typing pause run out.
func searchedFurther(t *testing.T, deps core.Deps, query string) *Model {
	t.Helper()
	m := tuitest.Resize(t, tuitest.Settle(t, New(deps, query)), 100, 30).(*Model)
	if !m.searchingFurther {
		t.Fatal("no search of the data source was scheduled")
	}
	_, cmd := m.Update(searchFurtherDueMsg{query: query})
	return tuitest.Drive(t, m, cmd).(*Model)
}

func TestSearchFindsPlayersBeyondTheCache(t *testing.T) {
	m := searchedFurther(t, foundDeps(t, fakePlayers{found: []models.FoundPlayer{ronaldo}}), "ronaldo")
	out := m.View()
	for _, want := range []string{"NOT DOWNLOADED", "Cristiano Ronaldo", "Al Nassr"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n---\n%s", want, out)
		}
	}
	if _, ok := m.FavoriteTarget(); ok {
		t.Error("a player who isn't cached can't be starred")
	}
}

func TestCachedPlayersAreNotOfferedAgain(t *testing.T) {
	haaland := models.FoundPlayer{Provider: "src", ID: "9", Name: "Erling Haaland"}
	m := searchedFurther(t, foundDeps(t, fakePlayers{found: []models.FoundPlayer{haaland}}), "haaland")
	if out := m.View(); strings.Contains(out, "NOT DOWNLOADED") {
		t.Errorf("Haaland is cached, so he isn't offered for download\n---\n%s", out)
	}
}

func TestShortQueriesStayLocal(t *testing.T) {
	m := tuitest.Settle(t, New(foundDeps(t, fakePlayers{}), "ro")).(*Model)
	if m.searchingFurther {
		t.Error("a two-letter query shouldn't reach the data source")
	}
}

func TestOpeningAFoundPlayerDownloadsTheirCoveredSeasons(t *testing.T) {
	deps := foundDeps(t, fakePlayers{found: []models.FoundPlayer{ronaldo}})
	m := searchedFurther(t, deps, "ronaldo")
	cmd := m.downloadCareer(careerMsg{player: ronaldo, seasons: []models.CareerSeason{
		{CompetitionID: "eng-premier-league", Year: "2025"}, // cached
		{CompetitionID: "eng-premier-league", Year: "2024"},
		{CompetitionID: "eng-fa-cup", Year: "2024"}, // a cup: no player stats
		{CompetitionID: "sau-pro-league", Year: "2024"},
		{CompetitionID: "eng-premier-league", Year: "2023"},
	}})
	got := refreshRequests(cmd)
	want := []core.RefreshRequest{{CompetitionID: "eng-premier-league", Seasons: []string{"2024", "2023"}}}
	if !slices.EqualFunc(got, want, func(a, b core.RefreshRequest) bool {
		return a.CompetitionID == b.CompetitionID && slices.Equal(a.Seasons, b.Seasons)
	}) {
		t.Errorf("requests %+v, want %+v", got, want)
	}
	if m.awaiting == nil {
		t.Fatal("not waiting for the player to land")
	}

	// The download lands and links the player to the source's ID.
	if err := deps.Repos.UpsertPlayer(deps.Ctx, models.Player{ID: "p-ronaldo", Name: "Cristiano Ronaldo", Gender: "men"}); err != nil {
		t.Fatal(err)
	}
	storagetest.SeedRoster(t, deps.Repos, "eng-manchester-united", "eng-premier-league", "eng-premier-league:2025",
		[]storagetest.PlayerRow{{ID: "p-ronaldo", Name: "Cristiano Ronaldo", Position: "F"}})
	if err := deps.Repos.PutExternalRef(deps.Ctx, "player", "p-ronaldo", "src", "7"); err != nil {
		t.Fatal(err)
	}
	_, cmd = m.Update(core.RefreshedMsg{CompetitionID: "eng-premier-league"})
	m = tuitest.Drive(t, m, cmd).(*Model)
	if m.awaiting != nil {
		t.Error("still waiting after the player landed")
	}
	if r, ok := m.SelectedResult(); !ok || r.Kind != KindPlayer || r.Player.ID != "p-ronaldo" {
		t.Errorf("cursor on %+v, want the newly cached player", r)
	}
	if strings.Contains(m.View(), "NOT DOWNLOADED") {
		t.Errorf("the cached player is still offered for download\n---\n%s", m.View())
	}
}

// A source's old squads can leave a player out: once the downloads are
// done, Browse says so rather than waiting forever.
func TestAFoundPlayerMissingFromTheSquadsIsReported(t *testing.T) {
	m := searchedFurther(t, foundDeps(t, fakePlayers{found: []models.FoundPlayer{ronaldo}}), "ronaldo")
	m.downloadCareer(careerMsg{player: ronaldo, seasons: []models.CareerSeason{{CompetitionID: "eng-premier-league", Year: "2024"}}})

	// The queue going idle before the downloads have even started says
	// nothing.
	if _, cmd := m.Update(core.RefreshQueueMsg{}); cmd != nil {
		t.Fatal("gave up before the downloads started")
	}
	m.Update(core.RefreshQueueMsg{Active: &core.RefreshRequest{CompetitionID: "eng-premier-league"}})
	_, cmd := m.Update(core.RefreshQueueMsg{})
	msg, ok := cmd().(notLocatedMsg)
	if !ok {
		t.Fatalf("got %T, want a miss", msg)
	}
	_, cmd = m.Update(msg)
	if m.awaiting != nil {
		t.Error("still waiting after the downloads ended")
	}
	if status, ok := tuitest.CapturedCmd(cmd).(core.StatusMsg); !ok || !strings.Contains(status.Text, "wasn't in the squads") {
		t.Errorf("status %+v doesn't say the player wasn't found", status)
	}
}

func refreshRequests(cmd tea.Cmd) []core.RefreshRequest {
	var out []core.RefreshRequest
	var walk func(tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				walk(c)
			}
		case core.RefreshRequestMsg:
			out = append(out, msg.Requests...)
		}
	}
	walk(cmd)
	return out
}
