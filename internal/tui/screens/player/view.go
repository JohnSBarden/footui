package player

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
)

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(th.Error.Render("Could not load this player: " + m.err.Error()))
	case m.loading:
		b.WriteString(th.Muted.Render("Loading…"))
	case len(m.competitions) == 0:
		b.WriteString(m.emptyState())
	default:
		if cards := m.statCards(); cards != "" {
			b.WriteString(cards)
			b.WriteString("\n\n")
		}
		b.WriteString(m.seasonsTable())
		b.WriteString("\n\n")
		b.WriteString(m.careerLine())
	}

	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme

	var meta []string
	if m.player.Position != "" {
		meta = append(meta, m.player.Position)
	}
	if m.player.Nationality != "" {
		meta = append(meta, m.player.Nationality)
	}

	line := th.Title.Render(m.player.Name)
	if len(meta) > 0 {
		line = lipgloss.JoinHorizontal(lipgloss.Center, line, th.Subtitle.Render("  "+strings.Join(meta, " · ")))
	}

	if len(m.competitions) == 0 {
		return line
	}

	tabs := make([]components.Tab, 0, len(m.competitions))
	for _, c := range m.competitions {
		tabs = append(tabs, components.Tab{
			Label: c.Name,
			Badge: "(" + strconv.Itoa(len(m.history[c.ID])) + ")",
		})
	}
	return line + "\n" + components.Tabs(th, tabs, m.compIdx)
}

// statCards summarize the selected season, with per-90 rates alongside the
// raw totals: 12 goals means something different in 900 minutes than in
// 3000, and the rate is what makes two seasons comparable.
func (m *Model) statCards() string {
	row, ok := m.SelectedRow()
	if !ok {
		return ""
	}
	s := row.Stats
	th := m.deps.Theme

	stats := []components.Stat{
		{Label: "Season", Value: components.SeasonName(row.Season), Hint: row.Team.Name},
		{Label: "Minutes", Value: strconv.Itoa(s.Minutes), Hint: strconv.Itoa(s.Appearances) + " apps"},
		{Label: "Goals", Value: strconv.Itoa(s.Goals), Hint: per90Hint(s.Goals, s.Minutes)},
		{Label: "Assists", Value: strconv.Itoa(s.Assists), Hint: per90Hint(s.Assists, s.Minutes)},
	}

	if s.XG != nil {
		// Goals minus xG says whether a season's return was finishing or
		// chance volume — the question a goal tally alone can't answer.
		delta := float64(s.Goals) - *s.XG
		style := m.deltaStyle(delta)
		stats = append(stats, components.Stat{
			Label: "xG", Value: components.Float(s.XG, 1),
			Hint:  components.SignedFloat(delta, 1) + " vs G",
			Style: &style,
		})
	}
	if s.XA != nil {
		delta := float64(s.Assists) - *s.XA
		style := m.deltaStyle(delta)
		stats = append(stats, components.Stat{
			Label: "xA", Value: components.Float(s.XA, 1),
			Hint:  components.SignedFloat(delta, 1) + " vs A",
			Style: &style,
		})
	}
	if s.Shots != nil {
		stats = append(stats, components.Stat{
			Label: "Shots", Value: components.Int(s.Shots),
			Hint: per90Hint(*s.Shots, s.Minutes),
		})
	}
	if s.KeyPasses != nil {
		stats = append(stats, components.Stat{
			Label: "Key passes", Value: components.Int(s.KeyPasses),
			Hint: per90Hint(*s.KeyPasses, s.Minutes),
		})
	}

	return components.StatCards(th, stats, m.width)
}

// per90Hint renders a count as a per-90-minutes rate, or empty when there
// aren't enough minutes for the rate to mean anything. Under ~90 minutes a
// single goal implies absurd rates, which would be noise, not insight.
func per90Hint(count, minutes int) string {
	if minutes < 90 {
		return ""
	}
	rate := float64(count) * 90 / float64(minutes)
	return strconv.FormatFloat(rate, 'f', 2, 64) + "/90"
}

func (m *Model) seasonsTable() string {
	th := m.deps.Theme
	rows := m.CurrentRows()
	if len(rows) == 0 {
		return th.Empty.Render("No seasons cached in this competition.")
	}

	cells := make([][]components.Cell, 0, len(rows))
	for _, r := range rows {
		s := r.Stats
		cells = append(cells, []components.Cell{
			components.Text(components.SeasonName(r.Season)),
			components.Text(r.Team.Name),
			components.Text(strconv.Itoa(s.Appearances)),
			components.Text(strconv.Itoa(s.Minutes)),
			components.Styled(strconv.Itoa(s.Goals), th.Accent.Bold(true)),
			components.Text(strconv.Itoa(s.Assists)),
			components.Text(components.Float(s.XG, 1)),
			components.Text(components.Float(s.XA, 1)),
			components.Text(components.Int(s.Shots)),
			components.Text(components.Int(s.KeyPasses)),
		})
	}

	cols := m.columns()
	for i := range cells {
		cells[i] = components.TrimRow(cells[i], len(cols))
	}

	return components.Table(components.TableOpts{
		Columns: cols,
		Rows:    cells,
		Cursor:  m.cursor,
		MaxRows: m.tableHeight(),
		Theme:   th,
	})
}

// wantTeamCol keeps club names distinguishable — "Manchester United" and
// "Manchester City" must never both render as "Manchester …". It never
// shrinks; see components.FitColumns.
const wantTeamCol = 22

func (m *Model) columns() []components.Column {
	base := []components.Column{
		{Title: "Season", Width: 8},
		{Title: "Team", Width: wantTeamCol},
		{Title: "Apps", Width: 5, Right: true},
		{Title: "Mins", Width: 5, Right: true},
		{Title: "G", Width: 4, Right: true},
		{Title: "A", Width: 4, Right: true},
	}

	return components.FitColumns(m.width, base, 0, [][]components.Column{
		{
			{Title: "xG", Width: 6, Right: true},
			{Title: "xA", Width: 6, Right: true},
		},
		{
			{Title: "Sh", Width: 4, Right: true},
			{Title: "KP", Width: 4, Right: true},
		},
	})
}

// careerLine totals the selected competition. It is rendered below the
// table rather than as a final row so it can't be mistaken for a season.
func (m *Model) careerLine() string {
	th := m.deps.Theme
	comp, ok := m.CurrentCompetition()
	if !ok {
		return ""
	}
	c := m.CareerInCurrentCompetition()
	if c.Seasons == 0 {
		return ""
	}

	parts := []string{
		plural(c.Seasons, "season"),
		plural(c.Appearances, "app"),
		strconv.Itoa(c.Minutes) + " mins",
		plural(c.Goals, "goal"),
		plural(c.Assists, "assist"),
	}
	if c.HasShots {
		parts = append(parts, strconv.Itoa(c.Shots)+" shots", strconv.Itoa(c.KeyPasses)+" key passes")
	}
	if c.HasXG {
		parts = append(parts,
			strconv.FormatFloat(c.XG, 'f', 1, 64)+" xG",
			strconv.FormatFloat(c.XA, 'f', 1, 64)+" xA")
	}

	return th.PanelTitle.Render("Career in "+comp.Name+": ") +
		th.Muted.Render(strings.Join(parts, " · "))
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func (m *Model) tableHeight() int {
	const chrome = 16
	h := m.height - chrome
	if h < 5 {
		return 5
	}
	return h
}

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

func (m *Model) emptyState() string {
	th := m.deps.Theme
	return th.Empty.Render("No cached seasons for "+m.player.Name+".") + "\n\n" +
		th.Muted.Render("Run  ") +
		th.Accent.Render("footy refresh") +
		th.Muted.Render("  for a competition this player appeared in.")
}
