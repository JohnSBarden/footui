package team

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/scorestrip"
)

// recentGames is how many games the form line is read from; the Games
// screen (m) lists more.
const recentGames = 5

// recentLoadedMsg carries the club's last few finished games in every
// competition, and the scores along the top.
type recentLoadedMsg struct {
	matches []models.Match
	strip   scorestrip.Strip
}

// loadRecent fetches the form line's games and the scores from the cache.
func (m *Model) loadRecent() tea.Cmd {
	deps, teamID := m.deps, m.team.ID
	return func() tea.Msg {
		matches, err := deps.Repos.RecentMatches(deps.Ctx, teamID, recentGames)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return recentLoadedMsg{matches: matches, strip: loadStrip(deps, teamID)}
	}
}

// loadStrip is the club's schedule either side of today. The strip is a
// nicety, so a failure leaves it empty rather than failing the screen.
func loadStrip(deps core.Deps, teamID string) scorestrip.Strip {
	t := now()
	ms, err := scorestrip.ForTeam(deps, teamID, t)
	if err != nil {
		return scorestrip.Strip{}
	}
	strip, err := scorestrip.Build(deps, ms, t)
	if err != nil {
		return scorestrip.Strip{}
	}
	strip.ShowComp = true
	return strip
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

// recentStrip is the club's form over its last five games, under the
// scores along the top, which carry the games themselves.
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
	if m.recentLoaded && len(m.recent) == 0 && !m.fetching && !m.queued {
		lines = append(lines, th.Empty.Render("No recent games cached.")+
			th.Muted.Render("  Run  ")+th.Accent.Render("footui refresh --team "+teamArg(m.team))+
			th.Muted.Render("  to fetch them."))
	}
	return strings.Join(lines, "\n")
}

// teamArg is the club as `footui refresh --team` takes it.
func teamArg(t models.Team) string {
	arg := strings.ToLower(t.Name)
	if strings.Contains(arg, " ") {
		return `"` + arg + `"`
	}
	return arg
}
