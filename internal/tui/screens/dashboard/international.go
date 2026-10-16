package dashboard

import (
	"fmt"
	"strings"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// recentResults is how many results a nation card lists.
const recentResults = 3

// loadTournament builds a starred international competition's card from
// its newest season: during the group stage, the group of a starred
// nation (or the first); once the knockouts have begun, the latest round.
func loadTournament(deps core.Deps, comp models.Competition) (Card, error) {
	c := Card{Kind: models.FavoriteCompetition, ID: comp.ID, Group: GroupTournaments, Competition: comp}
	seasons, err := deps.Repos.ListSeasons(deps.Ctx, comp.ID)
	if err != nil || len(seasons) == 0 {
		return c, err
	}
	c.Season = seasons[0]
	c.LastSync = lastSync(deps, comp.ID, c.Season.ID)
	stats, err := deps.Repos.Standings(deps.Ctx, comp.ID, c.Season.ID)
	if err != nil {
		return c, err
	}
	matches, err := deps.Repos.MatchesForCompetitionSeason(deps.Ctx, comp.ID, c.Season.ID)
	if err != nil {
		return c, err
	}
	ids := make([]string, 0, len(stats)+2*len(matches))
	for _, s := range stats {
		ids = append(ids, s.TeamID)
	}
	for _, mt := range matches {
		ids = append(ids, mt.HomeTeamID, mt.AwayTeamID)
	}
	if c.Teams, err = deps.Repos.GetTeams(deps.Ctx, ids); err != nil {
		return c, err
	}

	knockouts := false
	for _, mt := range matches {
		if mt.Finished() && components.Stage(mt) != components.GroupStage {
			knockouts = true
		}
	}
	grouped := len(stats) > 0 && stats[0].GroupLabel != ""
	switch {
	case grouped && !knockouts:
		c.GroupLabel = stats[0].GroupLabel
		for _, s := range stats {
			if deps.Favorites.Has(models.FavoriteTeam, s.TeamID) && s.GroupLabel != "" {
				c.GroupLabel = s.GroupLabel
				break
			}
		}
		for _, s := range stats {
			if s.GroupLabel == c.GroupLabel && len(c.Table) < tableRows {
				c.Table = append(c.Table, StandingRow{Position: s.Position, Team: c.team(s.TeamID), Points: s.Points, Played: s.MatchesPlayed})
			}
		}
	case len(matches) > 0:
		// The round of the latest result, or of the first fixture when
		// nothing has been played.
		latest := -1
		for i, mt := range matches {
			if mt.Finished() {
				latest = i
			}
		}
		if latest < 0 {
			latest = 0
		}
		c.RoundLabel = components.Stage(matches[latest])
		for _, mt := range matches {
			if components.Stage(mt) == c.RoundLabel {
				c.Results = append(c.Results, mt)
			}
		}
		if len(c.Results) > tableRows {
			c.Results = c.Results[len(c.Results)-tableRows:]
		}
	default:
		for i, s := range stats {
			if i < tableRows {
				c.Table = append(c.Table, StandingRow{Position: s.Position, Team: c.team(s.TeamID), Points: s.Points, Played: s.MatchesPlayed})
			}
		}
	}
	return c, nil
}

// loadNation builds a starred nation's card from its matches under the
// filter saved on its screen, so the card and the screen agree.
func loadNation(deps core.Deps, team models.Team) (Card, error) {
	c := Card{Kind: models.FavoriteTeam, ID: team.ID, Group: GroupNations, Team: team}
	filter, err := deps.Repos.GetTeamMatchFilter(deps.Ctx, team.ID)
	if err != nil {
		return c, err
	}
	c.Filter = filter
	matches, err := deps.Repos.MatchesForTeam(deps.Ctx, team.ID, filter, nil)
	if err != nil {
		return c, err
	}
	c.Summary = models.SummarizeMatches(team.ID, matches)
	ids := []string{}
	compIDs := []string{}
	for _, mt := range matches { // newest first
		if mt.UpdatedAt.After(c.Updated) {
			c.Updated = mt.UpdatedAt
		}
		switch {
		case mt.Finished() && len(c.Recent) < recentResults:
			c.Recent = append(c.Recent, mt)
		case mt.Status == models.MatchScheduled && !mt.Kickoff.Before(now()):
			next := mt
			c.Next = &next // the last one seen is the soonest
		default:
			continue
		}
		ids = append(ids, mt.HomeTeamID, mt.AwayTeamID)
		compIDs = append(compIDs, mt.CompetitionID)
	}
	if c.Teams, err = deps.Repos.GetTeams(deps.Ctx, ids); err != nil {
		return c, err
	}
	if c.Comps, err = deps.Repos.GetCompetitions(deps.Ctx, compIDs); err != nil {
		return c, err
	}
	return c, nil
}

// team names a side in a card's match lines.
func (c Card) team(id string) models.Team {
	if t, ok := c.Teams[id]; ok {
		return t
	}
	return models.Team{ID: id, Name: id}
}

func (c Card) comp(id string) string {
	if comp, ok := c.Comps[id]; ok {
		return comp.Abbrev()
	}
	return id
}

// inFlight reports whether a card's numbers are being refreshed now, or
// waiting to be.
func (m *Model) inFlight(c Card) (running, waiting bool) {
	keys := []string{}
	if c.Competition.ID != "" {
		keys = append(keys, core.RefreshRequest{CompetitionID: c.Competition.ID}.Key())
	}
	if c.Group == GroupNations {
		keys = append(keys, core.RefreshRequest{TeamID: c.Team.ID}.Key(), core.RefreshRequest{Nations: c.Team.Gender}.Key())
	}
	for _, k := range keys {
		running = running || m.active[k]
		waiting = waiting || m.pending[k]
	}
	return running, waiting
}

func (m *Model) tournamentLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	if c.Season.ID == "" {
		return append(lines, "", th.Empty.Render(components.Truncate("No seasons cached — press r.", inner)))
	}
	context := components.SeasonName(c.Season)
	switch {
	case c.GroupLabel != "":
		context = c.GroupLabel + " · " + context
	case c.RoundLabel != "":
		context = c.RoundLabel + " · " + context
	}
	lines = append(lines, th.Muted.Render(components.Truncate(context, inner)))
	maxPts := 0
	for _, r := range c.Table {
		maxPts = max(maxPts, r.Points)
	}
	for _, r := range c.Table {
		lines = append(lines, m.standingLine(c, r, maxPts, inner))
	}
	for _, mt := range c.Results {
		score := components.Score(mt)
		if score == "" {
			score = components.Kickoff(mt.Kickoff)[4:]
		}
		line := fmt.Sprintf("%s %s %s", c.team(mt.HomeTeamID).Name, score, c.team(mt.AwayTeamID).Name)
		style := th.TableCell
		if m.deps.Favorites.Has(models.FavoriteTeam, mt.HomeTeamID) || m.deps.Favorites.Has(models.FavoriteTeam, mt.AwayTeamID) {
			style = th.Accent.Bold(true)
		}
		lines = append(lines, style.Render(components.Truncate(line, inner)))
	}
	if len(c.Table) == 0 && len(c.Results) == 0 {
		lines = append(lines, "", th.Empty.Render(components.Truncate("Nothing cached yet — press r.", inner)))
	}
	for len(lines) < cardLines-1 {
		lines = append(lines, "")
	}
	return append(lines, m.freshness(c, inner))
}

// filterLabel says which of a nation's matches its card counts.
func filterLabel(f models.MatchFilter) string {
	switch {
	case f.IsAll():
		return "all matches"
	case f.Preset == models.PresetCompetitive:
		return "competitive"
	case f.Preset == models.PresetTournaments:
		return "tournaments"
	case len(f.Competitions) == 1:
		return "1 competition"
	}
	return fmt.Sprintf("%d competitions", len(f.Competitions))
}

func (m *Model) nationLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{
		m.heading(c, inner),
		th.Muted.Render(components.Truncate("national team · "+c.Team.Gender+" · "+filterLabel(c.Filter), inner)),
	}
	s := c.Summary
	if s.Played == 0 && c.Next == nil {
		return append(lines, "", th.Empty.Render(components.Truncate("No matches cached — press r.", inner)))
	}
	record := fmt.Sprintf("P%d W%d D%d L%d", s.Played, s.Won, s.Drawn, s.Lost)
	if s.Form != "" {
		record += "  " + s.Form
	}
	lines = append(lines, th.Accent.Bold(true).Render(components.Truncate(record, inner)))
	for _, mt := range c.Recent {
		style := th.TableCell
		switch mt.Result(c.Team.ID) {
		case "W":
			style = th.Positive
		case "L":
			style = th.Negative
		}
		opp := c.team(mt.Opponent(c.Team.ID)).Name
		score := components.ScoreFor(mt, c.Team.ID)
		left := mt.Kickoff.Local().Format("01-02") + " " + padRight(c.comp(mt.CompetitionID), 5) + " " + opp
		width := inner - len([]rune(score)) - 1
		lines = append(lines, th.TableCell.Render(padRight(components.Truncate(left, width), width))+" "+style.Render(score))
	}
	for len(lines) < cardLines-2 {
		lines = append(lines, "")
	}
	if c.Next != nil {
		next := "next: " + components.Kickoff(c.Next.Kickoff) + " v " + c.team(c.Next.Opponent(c.Team.ID)).Name
		lines = append(lines, th.Muted.Render(components.Truncate(next, inner)))
	} else {
		lines = append(lines, "")
	}
	return append(lines, m.freshness(c, inner))
}

func padRight(s string, w int) string {
	if n := len([]rune(s)); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}
