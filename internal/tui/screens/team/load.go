package team

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// loadHistory fetches every cached season for the team and groups it by
// competition. Grouping happens here, once, rather than on every render.
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
		// TeamHistory already orders by season descending, so each group
		// arrives most-recent-first and needs no further sorting.

		ordered := make([]models.Competition, 0, len(comps))
		for id := range history {
			comp, ok := comps[id]
			if !ok {
				comp = models.Competition{ID: id, Name: id}
			}
			ordered = append(ordered, comp)
		}
		sortCompetitions(ordered)

		return historyLoadedMsg{competitions: ordered, history: history}
	}
}

// loadColors fetches the club's curated brand colors. A club with none is
// the common case (the seed set is hand-curated), so a not-found result
// yields no message at all rather than an error the user would have to
// dismiss.
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

// loadHonours fetches the club's curated honours.
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

// loadSeasonDetail fetches the squad and the long-tail team stats for one
// competition-season.
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

// distinctRefs collects the unique competition and season IDs a team's
// history touches, so they can be resolved in two batch queries instead of
// one per row.
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

// sortCompetitions puts a team's league first (lowest tier number), then
// orders the rest by name, so the tab order is stable across refreshes
// rather than following map iteration.
func sortCompetitions(comps []models.Competition) {
	sort.Slice(comps, func(i, j int) bool {
		if comps[i].Tier != comps[j].Tier {
			// Tier 0 means "unknown" (a cup we have no metadata for) and
			// sorts last rather than first.
			if comps[i].Tier == 0 {
				return false
			}
			if comps[j].Tier == 0 {
				return true
			}
			return comps[i].Tier < comps[j].Tier
		}
		return comps[i].Name < comps[j].Name
	})
}

// selectInitial picks the tab and season row the screen opens on: the ones
// the user drilled in from when known, otherwise the most recent season of
// the first competition.
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
