package team

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// loadHistory groups by competition once here, not on every render.
func (m *Model) loadHistory() tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		stats, err := deps.Repos.TeamHistory(deps.Ctx, teamID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		compIDs, seasonIDs := distinctRefs(stats)
		comps, err := deps.Repos.GetCompetitions(deps.Ctx, compIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		seasons, err := deps.Repos.GetSeasons(deps.Ctx, seasonIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		history := make(map[string][]SeasonRow, len(comps))
		for _, s := range stats {
			season, ok := seasons[s.SeasonID]
			if !ok {
				season = models.Season{ID: s.SeasonID, CompetitionID: s.CompetitionID, Label: s.SeasonID}
			}
			history[s.CompetitionID] = append(history[s.CompetitionID], SeasonRow{Stats: s, Season: season})
		}
		// TeamHistory orders by season descending; groups need no sort.

		ordered := make([]models.Competition, 0, len(comps))
		for id := range history {
			comp, ok := comps[id]
			if !ok {
				comp = models.Competition{ID: id, Name: id}
			}
			ordered = append(ordered, comp)
		}
		components.SortCompetitions(ordered)

		return historyLoadedMsg{competitions: ordered, history: history}
	}
}

// loadColors sends no message when the club has no colours (the common
// case), rather than an error.
func (m *Model) loadColors() tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		colors, err := deps.Repos.GetTeamColors(deps.Ctx, teamID)
		if err != nil {
			return nil
		}
		return colorsLoadedMsg{colors: colors}
	}
}

func (m *Model) loadHonours() tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		honours, err := deps.Repos.TeamTrophies(deps.Ctx, teamID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return honoursLoadedMsg{honours: honours}
	}
}

func (m *Model) loadSeasonDetail(competitionID, seasonID string) tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		stats, err := deps.Repos.RosterForTeamSeason(deps.Ctx, teamID, competitionID, seasonID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		ids := make([]string, 0, len(stats))
		for _, s := range stats {
			ids = append(ids, s.PlayerID)
		}
		players, err := deps.Repos.GetPlayers(deps.Ctx, ids)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		rows := make([]RosterRow, 0, len(stats))
		for _, s := range stats {
			player, ok := players[s.PlayerID]
			if !ok {
				player = models.Player{ID: s.PlayerID, Name: s.PlayerID}
			}
			rows = append(rows, RosterRow{Stats: s, Player: player})
		}

		extras, err := deps.Repos.TeamSeasonStatExtras(deps.Ctx, teamID, competitionID, seasonID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		// A missing sync log entry is expected, not an error.
		var lastSync *models.SyncRun
		if run, err := deps.Repos.LatestSyncRun(deps.Ctx, competitionID, seasonID); err == nil {
			lastSync = &run
		}

		return seasonDetailLoadedMsg{
			seasonID: seasonID, competitionID: competitionID,
			roster: rows, extras: extras, lastSync: lastSync,
		}
	}
}

// distinctRefs lets refs resolve in two batch queries, not one per row.
func distinctRefs(stats []models.TeamSeasonStats) (compIDs, seasonIDs []string) {
	seenComp := map[string]bool{}
	seenSeason := map[string]bool{}
	for _, s := range stats {
		if !seenComp[s.CompetitionID] {
			seenComp[s.CompetitionID] = true
			compIDs = append(compIDs, s.CompetitionID)
		}
		if !seenSeason[s.SeasonID] {
			seenSeason[s.SeasonID] = true
			seasonIDs = append(seasonIDs, s.SeasonID)
		}
	}
	return compIDs, seasonIDs
}

func (m *Model) selectInitial() {
	m.compIdx = 0
	m.seasonIdx = 0

	if m.fromCompetitionID != "" {
		for i, c := range m.competitions {
			if c.ID == m.fromCompetitionID {
				m.compIdx = i
				break
			}
		}
	}
	if m.fromSeasonID != "" {
		for i, r := range m.CurrentRows() {
			if r.Season.ID == m.fromSeasonID {
				m.seasonIdx = i
				break
			}
		}
	}
}
