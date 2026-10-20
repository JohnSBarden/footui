package espn

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// searchLimit is how many players one search asks ESPN for; the search
// spans every sport, so some are dropped.
const searchLimit = 20

type searchResponse struct {
	Items []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Type        string `json:"type"`
		Sport       string `json:"sport"`
		Label       string `json:"label"`
	} `json:"items"`
}

// SearchPlayers implements provider.PlayerSearcher: footballers whose
// names match query, by ESPN's own relevance.
func (p *Provider) SearchPlayers(ctx context.Context, query string) ([]models.FoundPlayer, error) {
	q := url.Values{"query": {query}, "type": {"player"}, "limit": {fmt.Sprint(searchLimit)}}
	var resp searchResponse
	if err := p.client.search(ctx, q.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("espn: search %q: %w", query, err)
	}
	var out []models.FoundPlayer
	for _, it := range resp.Items {
		if it.Type != "player" || it.Sport != "soccer" || it.ID == "" {
			continue
		}
		out = append(out, models.FoundPlayer{Provider: ProviderName, ID: it.ID, Name: it.DisplayName, Context: it.Label})
	}
	return out, nil
}

// PlayerCareer implements provider.PlayerSearcher from AthleteCareer,
// newest first.
func (p *Provider) PlayerCareer(ctx context.Context, id string) ([]models.CareerSeason, error) {
	seasons, err := p.AthleteCareer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("espn: career of %s: %w", id, err)
	}
	out := make([]models.CareerSeason, 0, len(seasons))
	for _, s := range slices.Backward(seasons) {
		out = append(out, models.CareerSeason{CompetitionID: s.CompetitionID, Year: strconv.Itoa(s.Year)})
	}
	return out, nil
}
