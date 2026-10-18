package player

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// loadHistory fetches every cached season for the player and groups it by
// competition, resolving team, competition, and season names in three
// batch queries rather than one lookup per row.
func (m *Model) loadHistory() tea.Cmd {
	deps, player := m.deps, m.player
	playerID := player.ID
	return func() tea.Msg {
		stats, err := deps.Repos.PlayerHistory(deps.Ctx, playerID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		compIDs, seasonIDs, teamIDs := distinctRefs(stats)
		comps, err := deps.Repos.GetCompetitions(deps.Ctx, compIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		seasons, err := deps.Repos.GetSeasons(deps.Ctx, seasonIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		teams, err := deps.Repos.GetTeams(deps.Ctx, teamIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		colors, err := deps.Repos.TeamColorsFor(deps.Ctx, teamIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		history := make(map[string][]SeasonRow, len(comps))
		for _, s := range stats {
			season, ok := seasons[s.SeasonID]
			if !ok {
				season = models.Season{ID: s.SeasonID, CompetitionID: s.CompetitionID, Label: s.SeasonID}
			}
			team, ok := teams[s.TeamID]
			if !ok {
				team = models.Team{ID: s.TeamID, Name: s.TeamID}
			}
			history[s.CompetitionID] = append(history[s.CompetitionID],
				SeasonRow{Stats: s, Team: team, Season: season})
		}
		// PlayerHistory already orders by season descending.

		ordered := make([]models.Competition, 0, len(history))
		for id := range history {
			comp, ok := comps[id]
			if !ok {
				comp = models.Competition{ID: id, Name: id}
			}
			ordered = append(ordered, comp)
		}
		components.SortCompetitions(ordered)

		group, hasDetail, err := enrichHistory(deps, player, stats, history)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return historyLoadedMsg{competitions: ordered, history: history, colors: colors, group: group, hasDetail: hasDetail}
	}
}

func distinctRefs(stats []models.PlayerSeasonStats) (compIDs, seasonIDs, teamIDs []string) {
	seenComp := map[string]bool{}
	seenSeason := map[string]bool{}
	seenTeam := map[string]bool{}
	for _, s := range stats {
		if !seenComp[s.CompetitionID] {
			seenComp[s.CompetitionID] = true
			compIDs = append(compIDs, s.CompetitionID)
		}
		if !seenSeason[s.SeasonID] {
			seenSeason[s.SeasonID] = true
			seasonIDs = append(seasonIDs, s.SeasonID)
		}
		if !seenTeam[s.TeamID] {
			seenTeam[s.TeamID] = true
			teamIDs = append(teamIDs, s.TeamID)
		}
	}
	return compIDs, seasonIDs, teamIDs
}

// selectInitial picks the competition tab to open on: one the player
// appeared in for the club the user drilled in from, when that's known.
func (m *Model) selectInitial() {
	m.compIdx = 0
	m.cursor = 0
	if m.fromTeamID == "" {
		return
	}

	for i, c := range m.competitions {
		for _, row := range m.history[c.ID] {
			if row.Team.ID == m.fromTeamID {
				m.compIdx = i
				return
			}
		}
	}
}
