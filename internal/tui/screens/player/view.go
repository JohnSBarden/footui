package player

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Layout. As on the club screen, everything but the season table is
// drawn and measured first, and the table gets what's left.
const (
	// minTableLines is the least room the season table keeps under the
	// stat cards; the cards lose their boxes, then their later stats, first.
	minTableLines = 8
	// minTableRows is the fewest seasons the table shows before the lines
	// beneath it give up their room.
	minTableRows = 3
	// paneGap is the space between the two columns of a wide terminal.
	paneGap = 4
	// minSideWidth is the narrowest the stat cards' column gets beside
	// the table: room for three cards abreast.
	minSideWidth = 48
)

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	head := m.header()

	switch {
	case m.err != nil:
		return head + "\n\n" + th.Error.Render("Could not load this player: "+m.err.Error())
	case m.loading:
		return head + "\n\n" + th.Muted.Render("Loading…")
	case len(m.competitions) == 0:
		return head + "\n\n" + m.emptyState()
	}

	rest := m.height - components.Lines(head) - 1
	stats := m.stats()
	callout := ""
	if row, ok := m.SelectedRow(); ok && len(stats) > 0 {
		callout = m.callout(row)
	}

	// Wide: the seasons on the left, the selected one's numbers beside
	// them, where the eye goes after picking a row.
	tableWidth := components.ColumnsWidth(m.tableColumns(m.width))
	if side := m.width - tableWidth - paneGap; side >= minSideWidth && len(stats) > 0 {
		right := components.StatCards(th, stats, side)
		if callout != "" {
			right += "\n" + callout
		}
		return head + "\n\n" + components.SideBySide(paneGap, []int{tableWidth, side},
			m.seasonsColumn(rest, tableWidth), right)
	}

	var b strings.Builder
	b.WriteString(head + "\n\n")
	if len(stats) > 0 {
		budget := min(rest/3, rest-minTableLines) - components.Lines(callout)
		panel := components.StatPanel(th, stats, m.width, budget)
		if callout != "" {
			panel += "\n" + callout
		}
		b.WriteString(panel + "\n\n")
		rest -= components.Lines(panel) + 1
	}
	b.WriteString(m.seasonsColumn(rest, m.width))
	return b.String()
}

// seasonsColumn is the season table with its notes, the career totals
// and the detail's freshness, in at most lines lines and width columns.
// The table's rows come first: short of room for them, the career line
// shrinks to one line, then goes, and then the table's notes go too.
func (m *Model) seasonsColumn(lines, width int) string {
	full := m.careerLines(width)
	if st := m.detailStatus(); st != "" {
		full = append(full, st)
	}
	var short []string
	if career := m.careerLines(math.MaxInt); len(career) > 0 {
		short = []string{ansi.Truncate(career[0], width, "…")}
	}
	layouts := []struct {
		tail  []string
		notes bool
	}{{full, true}, {short, true}, {nil, true}, {nil, false}}

	rows := m.CurrentRows()
	_, _, notes := m.tableParts(rows, width)
	for i, l := range layouts {
		room := lines
		if len(l.tail) > 0 {
			room -= len(l.tail) + 1
		}
		need := 1 + components.TableLines(min(len(rows), minTableRows), len(rows))
		if l.notes {
			need += len(notes)
		}
		if room < need && i < len(layouts)-1 {
			continue
		}
		out := m.seasonsTable(room, l.notes)
		if len(l.tail) > 0 {
			out += "\n\n" + strings.Join(l.tail, "\n")
		}
		return out
	}
	return ""
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

// stats summarize the selected season, with per-90 rates alongside the
// raw totals: 12 goals means something different in 900 minutes than in
// 3000, and the rate is what makes two seasons comparable. They're listed
// most telling first, since a short terminal keeps only the first.
func (m *Model) stats() []components.Stat {
	row, ok := m.SelectedRow()
	if !ok {
		return nil
	}
	s := row.Stats
	if row.Detail != nil {
		return m.positionCards(row)
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

	return stats
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

// seasonsTable is the season-by-season table under its label, with the
// notes on its marks when withNotes, in at most lines lines.
func (m *Model) seasonsTable(lines int, withNotes bool) string {
	th := m.deps.Theme
	rows := m.CurrentRows()
	if len(rows) == 0 {
		return th.Empty.Render("No seasons cached in this competition.")
	}
	cols, cells, notes := m.tableParts(rows, m.width)
	if !withNotes {
		notes = nil
	}
	out := components.Label(th, "Season by season") + "\n" + components.Table(components.TableOpts{
		Columns: cols, Rows: cells, Cursor: m.cursor, Theme: th,
		MaxRows: components.TableRows(lines-1-len(notes), len(cells)),
	})
	if len(notes) > 0 {
		out += "\n" + strings.Join(notes, "\n")
	}
	return out
}

// tableColumns are the season table's columns at width.
func (m *Model) tableColumns(width int) []components.Column {
	cols, _, _ := m.tableParts(m.CurrentRows(), width)
	return cols
}

// tableParts builds the season table at width: its columns, its rows cut
// to them, and the notes explaining its marks.
func (m *Model) tableParts(rows []SeasonRow, width int) ([]components.Column, [][]components.Cell, []string) {
	th := m.deps.Theme
	if base, extra, cells, ok := m.positionTable(rows); ok {
		cols := components.FitColumns(width, base, 0, extra)
		for i := range cells {
			cells[i] = components.TrimRow(cells[i], len(cols))
		}
		return cols, cells, m.footnotes(rows)
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

	cols := m.columns(width)
	for i := range cells {
		cells[i] = components.TrimRow(cells[i], len(cols))
	}
	var notes []string
	if hasXG {
		notes = append(notes, th.Positive.Render("■")+th.Muted.Render(" beat the expected number  ")+
			th.Below.Render("■")+th.Muted.Render(" fell short"))
	}
	return cols, cells, notes
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

func (m *Model) columns(width int) []components.Column {
	base := []components.Column{
		{Title: "Season", Width: 8},
		{Title: "Team", Width: wantTeamCol},
		{Title: "Apps", Width: 5, Right: true},
		{Title: "Mins", Width: 5, Right: true},
		{Title: "G", Width: 4, Right: true},
		{Title: "A", Width: 4, Right: true},
	}

	return components.FitColumns(width, base, 0, [][]components.Column{
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

// careerLines total the selected competition, wrapped to width. They are
// rendered below the table rather than as a final row so they can't be
// mistaken for a season.
func (m *Model) careerLines(width int) []string {
	th := m.deps.Theme
	comp, ok := m.CurrentCompetition()
	if !ok {
		return nil
	}
	c := m.CareerInCurrentCompetition()
	if c.Seasons == 0 {
		return nil
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

	// Wrapped between figures, never inside one, and each line cut to
	// the width in case a single figure is wider still.
	lines := []string{th.PanelTitle.Render("Career in " + comp.Name + ": ")}
	for i, p := range parts {
		last := len(lines) - 1
		sep := ""
		if i > 0 {
			sep = " · "
		}
		if i > 0 && lipgloss.Width(lines[last])+len(sep)+lipgloss.Width(p) > width {
			lines = append(lines, "  ")
			last, sep = last+1, ""
		}
		lines[last] += th.Muted.Render(sep + p)
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, width, "…")
	}
	return lines
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
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
