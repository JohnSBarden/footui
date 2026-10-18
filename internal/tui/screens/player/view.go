package player

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/styles"
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
			b.WriteString("\n")
			if row, ok := m.SelectedRow(); ok {
				if c := m.callout(row); c != "" {
					b.WriteString(c)
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}
		b.WriteString(m.seasonsTable())
		b.WriteString("\n\n")
		b.WriteString(m.careerLine())
		if st := m.detailStatus(); st != "" {
			b.WriteString("\n")
			b.WriteString(st)
		}
	}

	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme

	row, hasRow := m.SelectedRow()
	colors := m.colors[row.Team.ID]
	fill, rule := styles.HeroStyles(th, colors.PrimaryHex, colors.SecondaryHex, m.deps.ThemeMode())

	var meta []string
	if hasRow && row.Team.Name != "" {
		meta = append(meta, row.Team.Name)
	}
	if m.player.Nationality != "" {
		meta = append(meta, m.player.Nationality)
	}
	metaLine := strings.Join(meta, " · ")
	if pos := positionBadge(m.player.Position); pos != "" {
		badge := fill.Reverse(true).Bold(true).Render(" " + pos + " ")
		if th.NoColor {
			badge = "[" + pos + "]"
		}
		metaLine = badge + fill.Render(" ") + fill.Render(metaLine)
	}

	line := components.Hero(components.HeroOpts{
		Fill: fill, Rule: rule, Width: m.width, NoColor: th.NoColor,
		Title: m.deps.Star(models.FavoritePlayer, m.player.ID, m.player.Name),
		Meta:  metaLine,
		Right: m.chart(),
	})

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

// positionBadge names the positions in an understat code ("F M S" is
// forward, midfield and substitute): every one played, since understat
// lists them alphabetically rather than by how often, so none can be
// called the main one. S is dropped, being no position. Another
// provider's wording passes through in capitals.
func positionBadge(code string) string {
	names := map[string]string{"F": "FWD", "M": "MID", "D": "DEF", "GK": "GK"}
	var out []string
	for _, f := range strings.Fields(code) {
		if f == "S" {
			continue
		}
		name, ok := names[f]
		if !ok {
			return strings.ToUpper(code)
		}
		out = append(out, name)
	}
	return strings.Join(out, "·")
}

// chartSeasons is how many seasons the band's chart covers.
const chartSeasons = 3

// chartBarWidth is the length of the chart's longest bar.
const chartBarWidth = 12

// chart sets the player's output against what was expected, a season a
// row, oldest first: goals against xG for a forward, assists against xA
// for everyone else. A competition without expected numbers gets none.
func (m *Model) chart() []string {
	rows := m.CurrentRows() // newest first
	if m.hasDetail {
		switch m.group {
		case playerstats.GK, playerstats.DEF:
			if c := m.cleanSheetChart(); c != nil {
				return c
			}
		}
	}
	// Without detail, what the player produces decides the chart: a
	// scorer's goals, a creator's assists. With it, the position does.
	goals, assists := 0, 0
	for _, r := range rows {
		goals += r.Stats.Goals
		assists += r.Stats.Assists
	}
	forward := goals > assists
	if m.hasDetail {
		forward = m.group == playerstats.FWD
	}
	type point struct {
		season           string
		actual, expected float64
	}
	var pts []point
	scale := 0.0
	for i := min(len(rows), chartSeasons) - 1; i >= 0; i-- {
		s := rows[i].Stats
		actual, expected := float64(s.Assists), s.XA
		if forward {
			actual, expected = float64(s.Goals), s.XG
		}
		if expected == nil {
			continue
		}
		pts = append(pts, point{components.SeasonName(rows[i].Season), actual, *expected})
		scale = max(scale, actual, *expected)
	}
	if len(pts) == 0 {
		return nil
	}
	title := "ASSISTS vs xA"
	if forward {
		title = "GOALS vs xG"
	}
	out := []string{title}
	for _, p := range pts {
		bar := components.Bar(p.actual, scale, chartBarWidth)
		out = append(out, fmt.Sprintf("%-7s %s %3d / %.1f", p.season,
			bar+strings.Repeat(" ", chartBarWidth-lipgloss.Width(bar)), int(p.actual), p.expected))
	}
	// The band right-aligns each line, so they're padded to one width to
	// keep the chart's columns straight.
	width := 0
	for _, l := range out {
		width = max(width, lipgloss.Width(l))
	}
	for i, l := range out {
		out[i] = l + strings.Repeat(" ", width-lipgloss.Width(l))
	}
	return out
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
	if row.Detail != nil {
		return components.StatCards(th, m.positionCards(row), m.width)
	}

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

	if base, extra, cells, ok := m.positionTable(rows); ok {
		cols := components.FitColumns(m.width, base, 0, extra)
		for i := range cells {
			cells[i] = components.TrimRow(cells[i], len(cols))
		}
		table := components.Label(th, "Season by season") + "\n" + components.Table(components.TableOpts{
			Columns: cols, Rows: cells, Cursor: m.cursor, MaxRows: m.tableHeight(), Theme: th,
		})
		if notes := m.footnotes(rows); len(notes) > 0 {
			table += "\n" + strings.Join(notes, "\n")
		}
		return table
	}

	cells := make([][]components.Cell, 0, len(rows))
	hasXG := false
	for _, r := range rows {
		s := r.Stats
		hasXG = hasXG || s.XG != nil || s.XA != nil
		cells = append(cells, []components.Cell{
			components.Text(components.SeasonName(r.Season)),
			components.Text(r.Team.Name),
			components.Text(strconv.Itoa(s.Appearances)),
			components.Text(strconv.Itoa(s.Minutes)),
			components.Styled(strconv.Itoa(s.Goals), toneOr(th, s.Goals, s.XG, th.Accent.Bold(true))),
			components.Styled(strconv.Itoa(s.Assists), toneOr(th, s.Assists, s.XA, th.TableCell)),
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

	table := components.Label(th, "Season by season") + "\n" + components.Table(components.TableOpts{
		Columns: cols,
		Rows:    cells,
		Cursor:  m.cursor,
		MaxRows: m.tableHeight(),
		Theme:   th,
	})
	if hasXG {
		table += "\n" + th.Positive.Render("■") + th.Muted.Render(" beat the expected number  ") +
			th.Below.Render("■") + th.Muted.Render(" fell short")
	}
	return table
}

// toneOr colours a count against its expected value when there is one,
// and falls back to style when there isn't.
func toneOr(th styles.Theme, actual int, expected *float64, style lipgloss.Style) lipgloss.Style {
	if expected == nil {
		return style
	}
	return components.Tone(th, float64(actual), *expected)
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

	// Cut to the terminal: a wrapped line would push the footer away.
	return ansi.Truncate(th.PanelTitle.Render("Career in "+comp.Name+": ")+
		th.Muted.Render(strings.Join(parts, " · ")), m.width, "…")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func (m *Model) tableHeight() int {
	// The stat cards, the table's label and legend, the career line and
	// the footer, plus the header band, which grows on a narrow terminal.
	const chrome = 18
	h := m.height - chrome - (lipgloss.Height(m.header()) - 2)
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
		th.Accent.Render("footui refresh") +
		th.Muted.Render("  for a competition this player appeared in.")
}
