package scorestrip

import (
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// ForTeams is each team's last results and next fixture, from the
// matches stored for it around now.
func ForTeams(deps core.Deps, teamIDs []string, now time.Time) ([]models.Match, error) {
	y := now.Year()
	years := []int{y - 1, y, y + 1}
	var out []models.Match
	for _, id := range teamIDs {
		ms, err := deps.Repos.MatchesForTeam(deps.Ctx, id, models.MatchFilter{}, years)
		if err != nil {
			return nil, err
		}
		out = append(out, TeamWindow(ms, now)...)
	}
	return out, nil
}

// ForTeam is the strip on a team's own page: its Schedule.
func ForTeam(deps core.Deps, teamID string, now time.Time) ([]models.Match, error) {
	y := now.Year()
	ms, err := deps.Repos.MatchesForTeam(deps.Ctx, teamID, models.MatchFilter{}, []int{y - 1, y, y + 1})
	if err != nil {
		return nil, err
	}
	return Schedule(ms, now), nil
}

// ForSeason is one season's latest round and its next.
func ForSeason(deps core.Deps, compID, seasonID string, now time.Time) ([]models.Match, error) {
	ms, err := deps.Repos.MatchesForCompetitionSeason(deps.Ctx, compID, seasonID)
	if err != nil {
		return nil, err
	}
	return Rounds(ms, now), nil
}

// Build is the strip for matches, with every side and competition named
// and every club's colours to hand.
func Build(deps core.Deps, matches []models.Match, now time.Time) (Strip, error) {
	ms := Ordered(matches)
	s := Strip{Matches: ms, Now: now}
	if len(ms) == 0 {
		return s, nil
	}
	teams, comps, err := core.MatchNames(deps, ms)
	if err != nil {
		return s, err
	}
	ids := make([]string, 0, len(teams))
	for id := range teams {
		ids = append(ids, id)
	}
	colors, err := deps.Repos.TeamColorsFor(deps.Ctx, ids)
	if err != nil {
		return s, err
	}
	s.Teams, s.Comps, s.Colors = teams, comps, colors
	return s, nil
}
