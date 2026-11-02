package team

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// lastGameZone is the card's clickable region.
const lastGameZone = "lastgame"

// minCardBody must remain below the Last game card for it to be drawn;
// the card goes before the score strip does.
const minCardBody = minStripBody

// lastGame is the club's last finished game in full, when its detail is
// stored.
type lastGame struct {
	match  models.Match
	detail models.MatchDetail
	teams  map[string]models.Team
	comp   string // the competition's name, "" when unknown
}

// loadLastGame reads the stored detail of the newest of recent. No detail
// is no card, not an error; nor is a game the source didn't have.
func loadLastGame(deps core.Deps, recent []models.Match) (*lastGame, error) {
	if len(recent) == 0 {
		return nil, nil
	}
	g := recent[0]
	d, err := deps.Repos.MatchDetail(deps.Ctx, g.ID)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, err
	case d.Empty():
		return nil, nil
	}
	teams, comps, err := core.MatchNames(deps, []models.Match{g})
	if err != nil {
		return nil, err
	}
	return &lastGame{match: g, detail: d, teams: teams, comp: comps[g.CompetitionID].Name}, nil
}

// LastGame is the game `e` opens: the newest finished one.
func (m *Model) LastGame() (models.Match, bool) {
	if len(m.recent) == 0 {
		return models.Match{}, false
	}
	return m.recent[0], true
}

func (m *Model) openLastGame() tea.Cmd {
	g, ok := m.LastGame()
	if !ok || m.deps.Nav.Match == nil {
		return nil
	}
	return core.Push(m.deps.Nav.Match(m.deps, g, m.team.ID))
}

func (lg *lastGame) name(id string) string {
	if t, ok := lg.teams[id]; ok && t.Name != "" {
		return t.Name
	}
	return id
}

// lastGameCard is the Last game card, or "" when there's nothing to show.
func (m *Model) lastGameCard() string {
	lg := m.last
	if lg == nil {
		return ""
	}
	th := m.theme
	g, d := lg.match, lg.detail
	width := min(m.width, 100)

	info := []string{}
	if lg.comp != "" {
		info = append(info, lg.comp)
	}
	info = append(info, g.Kickoff.Local().Format("Mon 2 Jan"))
	if d.Venue != "" {
		info = append(info, d.Venue)
	}
	label := components.Label(th, "Last game")
	hint := th.Muted.Render(keys.LastGame.Help().Key + " details")
	room := max(width-lipgloss.Width(label)-lipgloss.Width(hint)-4, 10)
	title := label + th.Muted.Render("  "+components.Truncate(strings.Join(info, " · "), room))
	title += strings.Repeat(" ", max(width-lipgloss.Width(title)-lipgloss.Width(hint), 2)) + hint

	score := th.TableCell.Bold(true).Render(lg.name(g.HomeTeamID) + "  " + components.Score(g) + "  " + lg.name(g.AwayTeamID))
	if scorers := components.TeamScorers(d.Events, lg.name(g.HomeTeamID), lg.name(g.AwayTeamID)); scorers != "" {
		room := width - lipgloss.Width(score) - 3
		if room >= 12 {
			score += "   " + th.Muted.Render(components.Truncate(scorers, room))
		}
	}
	lines := []string{title, score}

	side := models.SideAway
	if g.HomeTeamID == m.team.ID {
		side = models.SideHome
	}
	const legend = "   ● goal ■ card ▲ sub"
	axisWidth := width
	if width >= 80 && !th.NoColor {
		axisWidth = width - lipgloss.Width(legend)
	}
	if tl := components.Timeline(th, d.Events, side, axisWidth); tl != "" {
		if axisWidth < width {
			tl += th.Faint.Render(legend)
		}
		lines = append(lines, tl)
	}
	if s := m.cardStats(d); s != "" {
		lines = append(lines, s)
	}
	return components.Zone(lastGameZone, strings.Join(lines, "\n"))
}

// cardStats is the one line of numbers: possession as a split bar, then
// shots and shots on target, home first.
func (m *Model) cardStats(d models.MatchDetail) string {
	th := m.theme
	h, a := d.Home.Stats, d.Away.Stats
	var parts []string
	if h.Possession != nil && a.Possession != nil {
		bar := components.SplitBar(th, *h.Possession, *a.Possession, 18)
		parts = append(parts, th.Muted.Render("poss ")+
			th.TableCell.Render(fmt.Sprintf("%.0f%% ", *h.Possession))+bar+
			th.TableCell.Render(fmt.Sprintf(" %.0f%%", *a.Possession)))
	}
	pair := func(label string, x, y *int) {
		if x != nil && y != nil {
			parts = append(parts, th.Muted.Render(label+" ")+th.TableCell.Render(fmt.Sprintf("%d · %d", *x, *y)))
		}
	}
	pair("shots", h.Shots, a.Shots)
	pair("on target", h.ShotsOnTarget, a.ShotsOnTarget)
	return strings.Join(parts, "   ")
}
