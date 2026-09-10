package team

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
		b.WriteString(th.Error.Render("Could not load this team: " + m.err.Error()))
		b.WriteString("\n\n")
		b.WriteString(m.footer())
		return b.String()
	case m.loading:
		b.WriteString(th.Muted.Render("Loading…"))
		b.WriteString("\n\n")
		b.WriteString(m.footer())
		return b.String()
	case len(m.competitions) == 0:
		b.WriteString(m.emptyState())
		b.WriteString("\n\n")
		b.WriteString(m.footer())
		return b.String()
	}

	if cards := m.statCards(); cards != "" {
		b.WriteString(cards)
		b.WriteString("\n\n")
	}

	b.WriteString(m.seasonsTable())
	b.WriteString("\n\n")
	b.WriteString(m.squadTable())
	b.WriteString("\n\n")
	b.WriteString(m.footer())

	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme

	var meta []string
	if m.team.Country != "" {
		meta = append(meta, m.team.Country)
	}
	if m.team.Gender != "" {
		meta = append(meta, m.team.Gender)
	}
	if m.team.Venue != "" {
		meta = append(meta, m.team.Venue)
	}

	line := th.Title.Render(m.team.Name)
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

// statCards summarizes the selected season. Every figure comes from that
// one (competition, season) row — nothing here is aggregated across
// competitions.
func (m *Model) statCards() string {
	row, ok := m.CurrentSeasonRow()
	if !ok {
		return ""
	}
	s := row.Stats
	th := m.deps.Theme

	gdStyle := m.deltaStyle(float64(s.GoalDiff))
	stats := []components.Stat{
		{Label: "Position", Value: components.Int(s.Position), Hint: components.SeasonLabel(row.Season.Label)},
		{Label: "Points", Value: strconv.Itoa(s.Points), Hint: strconv.Itoa(s.MatchesPlayed) + " played"},
		{Label: "Record", Value: strconv.Itoa(s.Wins) + "-" + strconv.Itoa(s.Draws) + "-" + strconv.Itoa(s.Losses), Hint: "W-D-L"},
		{Label: "Goals", Value: strconv.Itoa(s.GoalsFor) + ":" + strconv.Itoa(s.GoalsAgainst), Hint: components.Signed(s.GoalDiff), Style: &gdStyle},
	}

	if s.XG != nil {
		// The gap between actual and expected goals is the single most
		// informative derived number on the screen, so it gets a card of
		// its own rather than being left for the reader to subtract.
		delta := float64(s.GoalsFor) - *s.XG
		style := m.deltaStyle(delta)
		stats = append(stats, components.Stat{
			Label: "xG", Value: components.Float(s.XG, 1),
			Hint:  components.SignedFloat(delta, 1) + " vs GF",
			Style: &style,
		})
	}
	if s.XGA != nil {
		delta := *s.XGA - float64(s.GoalsAgainst)
		style := m.deltaStyle(delta)
		stats = append(stats, components.Stat{
			Label: "xGA", Value: components.Float(s.XGA, 1),
			Hint:  components.SignedFloat(delta, 1) + " vs GA",
			Style: &style,
		})
	}
	if s.PossessionAvg != nil {
		stats = append(stats, components.Stat{Label: "Possession", Value: components.Float(s.PossessionAvg, 1) + "%"})
	}

	return components.StatCards(th, stats, m.width)
}

func (m *Model) seasonsTable() string {
	th := m.deps.Theme
	rows := m.CurrentRows()
	if len(rows) == 0 {
		return th.Empty.Render("No seasons cached for this competition.")
	}

	cells := make([][]components.Cell, 0, len(rows))
	for _, r := range rows {
		s := r.Stats
		cells = append(cells, []components.Cell{
			components.Text(components.SeasonLabel(r.Season.Label)),
			components.Text(components.Int(s.Position)),
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
		})
	}

	cursor := -1
	if m.focus == paneSeasons {
		cursor = m.seasonIdx
	}

	return m.paneTitle("Seasons", m.focus == paneSeasons) + "\n" +
		components.Table(components.TableOpts{
			Columns: []components.Column{
				{Title: "Season", Width: 8},
				{Title: "Pos", Width: 4, Right: true},
				{Title: "MP", Width: 3, Right: true},
				{Title: "W", Width: 3, Right: true},
				{Title: "D", Width: 3, Right: true},
				{Title: "L", Width: 3, Right: true},
				{Title: "GF", Width: 4, Right: true},
				{Title: "GA", Width: 4, Right: true},
				{Title: "GD", Width: 4, Right: true},
				{Title: "Pts", Width: 4, Right: true},
				{Title: "xG", Width: 6, Right: true},
				{Title: "xGA", Width: 6, Right: true},
			},
			Rows:    cells,
			Cursor:  cursor,
			MaxRows: m.paneHeight(),
			Theme:   th,
		})
}

func (m *Model) squadTable() string {
	th := m.deps.Theme

	row, hasSeason := m.CurrentSeasonRow()
	label := ""
	if hasSeason {
		label = " — " + components.SeasonLabel(row.Season.Label)
	}
	title := m.paneTitle("Squad"+label, m.focus == paneSquad)

	switch {
	case m.rosterLoading:
		return title + "\n" + th.Muted.Render("Loading squad…")
	case len(m.roster) == 0:
		return title + "\n" + th.Empty.Render("No squad cached for this season.")
	}

	cells := make([][]components.Cell, 0, len(m.roster))
	for _, r := range m.roster {
		s := r.Stats
		cells = append(cells, []components.Cell{
			components.Text(r.Player.Name),
			components.Text(r.Player.Position),
			components.Text(strconv.Itoa(s.Appearances)),
			components.Text(strconv.Itoa(s.Minutes)),
			components.Styled(strconv.Itoa(s.Goals), th.Accent.Bold(true)),
			components.Text(strconv.Itoa(s.Assists)),
			components.Text(components.Float(s.XG, 1)),
			components.Text(components.Float(s.XA, 1)),
		})
	}

	cursor := -1
	if m.focus == paneSquad {
		cursor = m.cursorSquad
	}

	return title + "\n" + components.Table(components.TableOpts{
		Columns: []components.Column{
			{Title: "Player", Width: m.playerColWidth()},
			{Title: "Pos", Width: 6},
			{Title: "Apps", Width: 5, Right: true},
			{Title: "Mins", Width: 5, Right: true},
			{Title: "G", Width: 4, Right: true},
			{Title: "A", Width: 4, Right: true},
			{Title: "xG", Width: 6, Right: true},
			{Title: "xA", Width: 6, Right: true},
		},
		Rows:    cells,
		Cursor:  cursor,
		MaxRows: m.paneHeight(),
		Theme:   th,
	})
}

func (m *Model) playerColWidth() int {
	// Everything except the player column, plus the seven separators.
	const fixed = 6 + 5 + 5 + 4 + 4 + 6 + 6 + 7
	w := m.width - fixed - 4
	if w < 14 {
		return 14
	}
	if w > 30 {
		return 30
	}
	return w
}

// paneTitle marks which table the cursor is in. The arrow marker means the
// focused pane is still identifiable without color.
func (m *Model) paneTitle(label string, focused bool) string {
	th := m.deps.Theme
	if focused {
		return th.PanelTitle.Render("▸ " + label)
	}
	return th.Muted.Render("  " + label)
}

// paneHeight splits the remaining vertical space between the two tables.
func (m *Model) paneHeight() int {
	const chrome = 18 // header, tabs, stat cards, both pane titles, footer
	h := (m.height - chrome) / 2
	if h < 4 {
		return 4
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
	return th.Empty.Render("No cached seasons for "+m.team.Name+".") + "\n\n" +
		th.Muted.Render("Run  ") +
		th.Accent.Render("footy refresh") +
		th.Muted.Render("  for a competition this team plays in.")
}

func (m *Model) footer() string {
	keys := []string{
		"↑/↓ row",
		"←/→ competition",
		"tab seasons/squad",
		"enter open",
		"esc back",
		"q quit",
	}
	return m.deps.Theme.Help.Render(strings.Join(keys, "  ·  "))
}
