package core

import "github.com/JohnSBarden/footui/internal/data/models"

// MatchNames resolves the matches' teams and competitions in two queries.
func MatchNames(deps Deps, matches []models.Match) (map[string]models.Team, map[string]models.Competition, error) {
	var teamIDs, compIDs []string
	seenTeam, seenComp := map[string]bool{}, map[string]bool{}
	for _, m := range matches {
		for _, id := range []string{m.HomeTeamID, m.AwayTeamID} {
			if !seenTeam[id] {
				seenTeam[id] = true
				teamIDs = append(teamIDs, id)
			}
		}
		if !seenComp[m.CompetitionID] {
			seenComp[m.CompetitionID] = true
			compIDs = append(compIDs, m.CompetitionID)
		}
	}
	teams, err := deps.Repos.GetTeams(deps.Ctx, teamIDs)
	if err != nil {
		return nil, nil, err
	}
	comps, err := deps.Repos.GetCompetitions(deps.Ctx, compIDs)
	if err != nil {
		return nil, nil, err
	}
	// A league cached without a short name gets it from the catalog.
	for id, c := range comps {
		if c.ShortName != "" {
			continue
		}
		for _, e := range deps.Catalog {
			if e.Competition.ID == id {
				c.ShortName = e.Competition.ShortName
				comps[id] = c
			}
		}
	}
	return teams, comps, nil
}
