package espn

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// searchServer serves a hand-written search and statistics log.
func searchServer(t *testing.T) *Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/site/search":
			if r.URL.Query().Get("query") != "tom sample" {
				t.Errorf("searched for %q", r.URL.Query().Get("query"))
			}
			_, _ = w.Write([]byte(`{"items": [
				{"id": "901", "displayName": "Tom Sample", "type": "player", "sport": "soccer", "label": "Spanish LALIGA - Example FC"},
				{"id": "902", "displayName": "Tom Sample", "type": "player", "sport": "basketball", "label": "NCAAM - Somewhere"},
				{"id": "903", "displayName": "Tom Sample Jr", "type": "player", "sport": "soccer", "label": ""}
			]}`))
		case "/core/all/athletes/901/statisticslog":
			_, _ = w.Write([]byte(`{"entries": [
				{"season": {"$ref": "http://x/v2/sports/soccer/leagues/eng.1/seasons/2019?lang=en"}},
				{"season": {"$ref": "http://x/v2/sports/soccer/leagues/eng.fa/seasons/2019?lang=en"}},
				{"season": {"$ref": "http://x/v2/sports/soccer/leagues/uefa.champions/seasons/2020?lang=en"}},
				{"season": {"$ref": "http://x/v2/sports/soccer/leagues/sau.1/seasons/2023?lang=en"}},
				{"season": {"$ref": "http://x/v2/sports/soccer/leagues/esp.1/seasons/2024?lang=en"}},
				{"season": {"$ref": "http://x/v2/sports/soccer/leagues/esp.1/seasons/2024?lang=en"}}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewAt(srv.URL)
}

func TestSearchPlayersKeepsFootballersOnly(t *testing.T) {
	got, err := searchServer(t).SearchPlayers(ctxBkgnd, "tom sample")
	if err != nil {
		t.Fatal(err)
	}
	want := []models.FoundPlayer{
		{Provider: ProviderName, ID: "901", Name: "Tom Sample", Context: "Spanish LALIGA - Example FC"},
		{Provider: ProviderName, ID: "903", Name: "Tom Sample Jr"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPlayerCareerIsNewestFirstAndKnownCompetitionsOnly(t *testing.T) {
	got, err := searchServer(t).PlayerCareer(ctxBkgnd, "901")
	if err != nil {
		t.Fatal(err)
	}
	want := []models.CareerSeason{
		{CompetitionID: "esp-la-liga", Year: "2024"},
		{CompetitionID: "eur-champions-league", Year: "2020"},
		{CompetitionID: "eng-fa-cup", Year: "2019"},
		{CompetitionID: "eng-premier-league", Year: "2019"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
