package league

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
)

// Fixed column widths for every non-team column, plus the single space
// between columns. The team column absorbs whatever width is left over, so
// the table fills a wide terminal without overflowing a narrow one.
const (
	posWidth    = 3
	smallWidth  = 3 // MP, W, D, L
	goalWidth   = 4 // GF, GA, GD, Pts
	xgWidth     = 6 // xG, xGA, xGD
	minTeamCol  = 12
	teamColSlop = 4 // leading indent + breathing room at the right edge
)

func (m *Model) columns() []components.Column {
	fixed := posWidth + smallWidth*4 + goalWidth*4 + xgWidth*3
	// 12 single-space separators between 13 columns.
	fixed += 12 + teamColSlop

	teamWidth := m.width - fixed
	if teamWidth < minTeamCol {
		teamWidth = minTeamCol
	}
	if teamWidth > 28 {
		teamWidth = 28
	}

	return []components.Column{
		{Title: "#", Width: posWidth, Right: true},
		{Title: "Team", Width: teamWidth},
		{Title: "MP", Width: smallWidth, Right: true},
		{Title: "W", Width: smallWidth, Right: true},
		{Title: "D", Width: smallWidth, Right: true},
		{Title: "L", Width: smallWidth, Right: true},
		{Title: "GF", Width: goalWidth, Right: true},
		{Title: "GA", Width: goalWidth, Right: true},
		{Title: "GD", Width: goalWidth, Right: true},
		{Title: "Pts", Width: goalWidth, Right: true},
		{Title: "xG", Width: xgWidth, Right: true},
		{Title: "xGA", Width: xgWidth, Right: true},
		{Title: "xGD", Width: xgWidth, Right: true},
	}
}

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(th.Error.Render("Could not load standings: " + m.err.Error()))
	case m.loading:
		b.WriteString(th.Muted.Render("Loading standings…"))
	case len(m.seasons) == 0:
		b.WriteString(m.emptyState("No seasons cached for this competition."))
	case len(m.rows) == 0:
		season, _ := m.currentSeason()
		b.WriteString(m.emptyState(fmt.Sprintf(
			"No standings cached for %s.", components.SeasonLabel(season.Label))))
	default:
		b.WriteString(m.table())
	}

	b.WriteString("\n\n")
	b.WriteString(m.footer())
	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme

	name := m.comp.Name
	if name == "" {
		name = m.compID
	}
	title := th.Title.Render(name)

	var meta []string
	if m.comp.Country != "" {
		meta = append(meta, m.comp.Country)
	}
	if m.comp.Tier > 0 {
		meta = append(meta, "tier "+strconv.Itoa(m.comp.Tier))
	}
	if m.comp.Gender != "" {
		meta = append(meta, m.comp.Gender)
	}

	line := title
	if len(meta) > 0 {
		line = lipgloss.JoinHorizontal(lipgloss.Center, title, th.Subtitle.Render("  "+strings.Join(meta, " · ")))
	}

	if seasonBar := m.seasonBar(); seasonBar != "" {
		if freshness := m.freshnessNote(); freshness != "" {
			seasonBar += "   " + freshness
		}
		return line + "\n" + seasonBar
	}
	return line
}

// freshnessNote says when this season was last refreshed. A failed attempt
// is reported as a failure rather than silently showing the older
// successful time, because "we tried and it broke" is what the user needs
// to act on.
func (m *Model) freshnessNote() string {
	th := m.deps.Theme
	if m.lastSync == nil {
		return ""
	}

	ago := components.Freshness(m.lastSync.FinishedAt, time.Now())
	if ago == "" {
		return ""
	}
	if !m.lastSync.Succeeded() {
		return th.Negative.Render("refresh failed " + ago)
	}
	return th.Muted.Render("updated " + ago)
}

// seasonBar renders the season selector. It's shown even with a single
// cached season so the user can always see which season the numbers are
// from, rather than having to infer it.
func (m *Model) seasonBar() string {
	th := m.deps.Theme
	if len(m.seasons) == 0 {
		return ""
	}

	parts := make([]string, 0, len(m.seasons))
	for i, s := range m.seasons {
		label := components.SeasonLabel(s.Label)
		if i == m.seasonIdx {
			parts = append(parts, th.Accent.Bold(true).Render("["+label+"]"))
		} else {
			parts = append(parts, th.Muted.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, " ")
}

func (m *Model) table() string {
	th := m.deps.Theme
	rows := make([][]components.Cell, 0, len(m.rows))

	for _, r := range m.rows {
		s := r.Stats

		xgd := ""
		var xgdCell components.Cell
		if s.XG != nil && s.XGA != nil {
			diff := *s.XG - *s.XGA
			xgd = components.SignedFloat(diff, 1)
			xgdCell = components.Styled(xgd, m.deltaStyle(diff))
		} else {
			xgdCell = components.Text(components.Missing)
		}

		rows = append(rows, []components.Cell{
			components.Text(components.Int(s.Position)),
			components.Text(r.Team.Name),
			components.Text(strconv.Itoa(s.MatchesPlayed)),
			components.Text(strconv.Itoa(s.Wins)),
			components.Text(strconv.Itoa(s.Draws)),
			components.Text(strconv.Itoa(s.Losses)),
			components.Text(strconv.Itoa(s.GoalsFor)),
			components.Text(strconv.Itoa(s.GoalsAgainst)),
			components.Styled(components.Signed(s.GoalDiff), m.deltaStyle(float64(s.GoalDiff))),
			components.Styled(strconv.Itoa(s.Points), th.Accent.Bold(true)),
			components.Text(components.Float(s.XG, 1)),
			components.Text(components.Float(s.XGA, 1)),
			xgdCell,
		})
	}

	return components.Table(components.TableOpts{
		Columns: m.columns(),
		Rows:    rows,
		Cursor:  m.cursor,
		MaxRows: m.tableHeight(),
		Theme:   th,
	})
}

// deltaStyle colors a signed value by direction. Under NO_COLOR the theme's
// positive/negative styles are identical, and the explicit +/- sign
// components.Signed already emits is what carries the meaning.
func (m *Model) deltaStyle(v float64) lipgloss.Style {
	switch {
	case v > 0:
		return m.deps.Theme.Positive
	case v < 0:
		return m.deps.Theme.Negative
	default:
		return m.deps.Theme.Muted
	}
}

// tableHeight is how many standings rows fit, leaving room for the header
// block, column headers, footer, and the table's own "showing x-y of z"
// line.
func (m *Model) tableHeight() int {
	const chrome = 9
	h := m.height - chrome
	if h < 5 {
		return 5
	}
	return h
}

func (m *Model) emptyState(reason string) string {
	th := m.deps.Theme
	return th.Empty.Render(reason) + "\n\n" +
		th.Muted.Render("Run  ") +
		th.Accent.Render("footy refresh --competition "+m.compID) +
		th.Muted.Render("  to populate it.")
}

func (m *Model) footer() string {
	th := m.deps.Theme
	keys := []string{
		"↑/↓ team",
		"←/→ season",
		"enter open",
		"esc back",
		"q quit",
	}
	return th.Help.Render(strings.Join(keys, "  ·  "))
}
