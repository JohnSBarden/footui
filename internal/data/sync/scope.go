package sync

import "github.com/JohnSBarden/footui/internal/data/models"

// Scope is the current set of competitions footui can actually sync. This
// is intentionally much smaller than the full v1 target (top-2-divisions
// of England/USA/France/Germany/Italy/Spain plus each country's top-tier
// women's league, per the design doc): it's exactly the six leagues the
// understat.com provider covers. IDs here must match the leagueKeys table
// in internal/data/provider/understat/mapper.go — extending real coverage
// (a new provider, or understat.com adding a league) means adding a row
// here and a matching provider-side entry, not restructuring this list.
var Scope = []models.Competition{
	{ID: "eng-premier-league", Name: "Premier League", Country: "England", Tier: 1, Gender: "men"},
	{ID: "esp-la-liga", Name: "La Liga", Country: "Spain", Tier: 1, Gender: "men"},
	{ID: "ger-bundesliga", Name: "Bundesliga", Country: "Germany", Tier: 1, Gender: "men"},
	{ID: "ita-serie-a", Name: "Serie A", Country: "Italy", Tier: 1, Gender: "men"},
	{ID: "fra-ligue-1", Name: "Ligue 1", Country: "France", Tier: 1, Gender: "men"},
	{ID: "rus-premier-league", Name: "Russian Premier League", Country: "Russia", Tier: 1, Gender: "men"},
}

// FindCompetition returns the in-Scope competition matching id, or false.
func FindCompetition(id string) (models.Competition, bool) {
	for _, c := range Scope {
		if c.ID == id {
			return c, true
		}
	}
	return models.Competition{}, false
}
