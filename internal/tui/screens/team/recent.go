package team

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// recentGames is how many games the strip at the top of the screen shows;
// the Games screen (m) lists more.
const recentGames = 5

// recentLoadedMsg carries the club's last few finished games in every
// competition, with what names their opponents and competitions.
type recentLoadedMsg struct {
	matches []models.Match
	teams   map[string]models.Team
	comps   map[string]models.Competition
}

// loadRecent fetches the strip's games from the cache.
func (m *Model) loadRecent() tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		matches, err := deps.Repos.RecentMatches(deps.Ctx, teamID, recentGames)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		teams, comps, err := core.MatchNames(deps, matches)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return recentLoadedMsg{matches: matches, teams: teams, comps: comps}
	}
}

// checkRecent asks, once on open, whether the club's recent games need
// fetching, and queues the club's refresh if they do. Asking costs no
// request; a club whose games are current queues nothing.
func (m *Model) checkRecent() tea.Cmd {
	due, club := m.deps.ClubDue, m.team
	if due == nil || club.National {
		return nil
	}
	return func() tea.Msg {
		why, ok := due(club)
		if !ok {
			return nil
		}
		return core.RefreshRequestMsg{Requests: []core.RefreshRequest{
			{TeamID: club.ID, Name: club.Name, Why: why, Automatic: true},
		}}
	}
}

// recentStrip is the club's form and last five games, under the header.
func (m *Model) recentStrip() string {
	th := m.theme
	title := components.Label(th, "Recent")
	if len(m.recent) > 0 {
		summary := models.SummarizeMatches(m.team.ID, m.recent)
		title += "  " + components.FormChips(th, summary.Form)
	}
	hint := th.Muted.Render("  " + keys.Matches.Help().Key + " all games")
	var note string
	switch {
	case m.fetching:
		note = th.Status.Render("  fetching recent games…")
	case m.queued:
		note = th.Status.Render("  recent games queued")
	}
	lines := []string{title + hint + note}
	switch {
	case !m.recentLoaded:
		lines = append(lines, th.Muted.Render("Loading…"))
	case len(m.recent) == 0 && !m.fetching && !m.queued:
		lines = append(lines, th.Empty.Render("No recent games cached.")+
			th.Muted.Render("  Run  ")+th.Accent.Render("footui refresh --team "+teamArg(m.team))+
			th.Muted.Render("  to fetch them."))
	case len(m.recent) > 0:
		lines = append(lines, m.recentTable())
	}
	return strings.Join(lines, "\n")
}

// recentHeight is how many lines the strip and the gap beneath it take,
// which the tables below give up so the screen still fits.
func (m *Model) recentHeight() int {
	return lipgloss.Height(m.recentStrip()) + 1
}

func (m *Model) recentTable() string {
	th := m.theme
	cols := append(components.MatchColumns(22), components.Column{Title: "", Width: components.MatchXGWidth})
	rows := make([][]components.Cell, 0, len(m.recent))
	for _, mt := range m.recent {
		rows = append(rows, components.MatchCells(th, mt, m.team.ID, m.recentComp(mt.CompetitionID),
			m.recentTeam(mt.Opponent(m.team.ID)), components.MatchXGBar(th, mt, m.team.ID)))
	}
	return components.Table(components.TableOpts{Columns: cols, Rows: rows, Cursor: -1, Theme: th})
}

func (m *Model) recentComp(id string) models.Competition {
	if c, ok := m.recentComps[id]; ok {
		return c
	}
	return models.Competition{ID: id, Name: id}
}

func (m *Model) recentTeam(id string) string {
	if t, ok := m.recentTeams[id]; ok {
		return t.Name
	}
	return id
}

// teamArg is the club as `footui refresh --team` takes it.
func teamArg(t models.Team) string {
	arg := strings.ToLower(t.Name)
	if strings.Contains(arg, " ") {
		return `"` + arg + `"`
	}
	return arg
}
