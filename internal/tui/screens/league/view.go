package league

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// Column widths are fixed so digits line up down a column.
const (
	posWidth   = 3
	smallWidth = 3 // MP, W, D, L
	goalWidth  = 4 // GF, GA, GD, Pts
	xgWidth    = 6 // xG, xGA, xGD
	// wantTeamCol fits "Borussia Mönchengladbach" after the dot. It never
	// shrinks; see components.FitColumns.
	wantTeamCol    = 26
	pointsBarWidth = 10
	xgdHalf        = 6
)

func (m *Model) columns() []components.Column {
	base := []components.Column{
		{Title: "", Width: 1},
		{Title: "#", Width: posWidth, Right: true},
		{Title: "TEAM", Width: wantTeamCol},
		{Title: "MP", Width: smallWidth, Right: true},
		{Title: "W", Width: smallWidth, Right: true},
		{Title: "D", Width: smallWidth, Right: true},
		{Title: "L", Width: smallWidth, Right: true},
		{Title: "GF", Width: goalWidth, Right: true},
		{Title: "GA", Width: goalWidth, Right: true},
		{Title: "GD", Width: goalWidth, Right: true},
		{Title: "PTS", Width: goalWidth, Right: true},
	}

	var groups [][]components.Column
	for _, g := range m.extraGroups() {
		groups = append(groups, g.columns)
	}
	return components.FitColumns(m.width, base, 0, groups)
}

// extraGroup is optional columns shown or dropped together.
type extraGroup struct {
	columns []components.Column
	cells   func(s models.TeamSeasonStats) []components.Cell
}

// extraGroups are ordered as dropped on a narrow terminal. Columns with
// no values are never shown.
func (m *Model) extraGroups() []extraGroup {
	var groups []extraGroup
	has := func(f func(models.TeamSeasonStats) bool) bool {
		for _, r := range m.rows {
			if f(r.Stats) {
				return true
			}
		}
		return false
	}
	if has(func(s models.TeamSeasonStats) bool { return s.XG != nil }) {
		// xG and xGA go together; xGD derives from them.
		groups = append(groups,
			extraGroup{
				columns: []components.Column{
					{Title: "xG", Width: xgWidth, Right: true},
					{Title: "xGA", Width: xgWidth, Right: true},
				},
				cells: func(s models.TeamSeasonStats) []components.Cell {
					return []components.Cell{
						components.Text(components.Float(s.XG, 1)),
						components.Text(components.Float(s.XGA, 1)),
					}
				},
			},
			extraGroup{
				columns: []components.Column{{Title: "xGD", Width: xgWidth, Right: true}},
				cells: func(s models.TeamSeasonStats) []components.Cell {
					if s.XG == nil || s.XGA == nil {
						return []components.Cell{components.Text(components.Missing)}
					}
					diff := *s.XG - *s.XGA
					return []components.Cell{components.Styled(components.SignedFloat(diff, 1), m.deltaStyle(diff))}
				},
			})
		return append(groups, m.pointsBarGroup(), m.xgdBarGroup())
	}
	if has(func(s models.TeamSeasonStats) bool { return s.PossessionAvg != nil }) {
		groups = append(groups, extraGroup{
			columns: []components.Column{{Title: "Poss", Width: xgWidth, Right: true}},
			cells: func(s models.TeamSeasonStats) []components.Cell {
				if s.PossessionAvg == nil {
					return []components.Cell{components.Text(components.Missing)}
				}
				return []components.Cell{components.Text(strconv.FormatFloat(*s.PossessionAvg, 'f', 1, 64) + "%")}
			},
		})
	}
	if has(func(s models.TeamSeasonStats) bool { return s.Shots != nil }) {
		groups = append(groups, extraGroup{
			columns: []components.Column{
				{Title: "Sh", Width: goalWidth, Right: true},
				{Title: "SoT", Width: goalWidth, Right: true},
			},
			cells: func(s models.TeamSeasonStats) []components.Cell {
				return []components.Cell{
					components.Text(components.Int(s.Shots)),
					components.Text(components.Int(s.ShotsOnTarget)),
				}
			},
		})
	}
	return append(groups, m.pointsBarGroup())
}

func (m *Model) pointsBarGroup() extraGroup {
	top := 0
	for _, r := range m.rows {
		top = max(top, r.Stats.Points)
	}
	return extraGroup{
		columns: []components.Column{{Title: "", Width: pointsBarWidth}},
		cells: func(s models.TeamSeasonStats) []components.Cell {
			return []components.Cell{components.Styled(
				components.Bar(float64(s.Points), float64(top), pointsBarWidth), m.deps.Theme.Accent.Faint(true))}
		},
	}
}

func (m *Model) xgdBarGroup() extraGroup {
	th := m.deps.Theme
	maxAbs := 0.0
	for _, r := range m.rows {
		if r.Stats.XG != nil && r.Stats.XGA != nil {
			maxAbs = max(maxAbs, math.Abs(*r.Stats.XG-*r.Stats.XGA))
		}
	}
	return extraGroup{
		columns: []components.Column{{Title: "−  xGD  +", Width: 2*xgdHalf + 1}},
		cells: func(s models.TeamSeasonStats) []components.Cell {
			if s.XG == nil || s.XGA == nil {
				return []components.Cell{components.Text("")}
			}
			neg, pos := components.Diverging(*s.XG-*s.XGA, maxAbs, xgdHalf)
			return []components.Cell{components.Text(
				strings.Repeat(" ", xgdHalf-lipgloss.Width(neg)) + th.Below.Render(neg) +
					th.Faint.Render("┃") + th.Info.Render(pos))}
		},
	}
}

// zoneCell falls back to a letter without colour.
func (m *Model) zoneCell(pos *int) components.Cell {
	th := m.deps.Theme
	season, _ := m.currentSeason()
	p := 0
	if pos != nil {
		p = *pos
	}
	z := zoneFor(m.compID, startYear(season.Label), p)
	if th.NoColor {
		return components.Text(map[zone]string{zoneNone: " ", zoneUCL: "C", zoneUEL: "E", zoneDown: "↓"}[z])
	}
	switch z {
	case zoneUCL:
		return components.Styled("▌", th.Info)
	case zoneUEL:
		return components.Styled("▌", th.Below)
	case zoneDown:
		return components.Styled("▌", th.Negative)
	}
	return components.Text(" ")
}

func (m *Model) zoneLegend() string {
	th := m.deps.Theme
	season, ok := m.currentSeason()
	if !ok || !hasZones(m.compID, startYear(season.Label)) {
		return ""
	}
	if th.NoColor {
		return "C Champions L.  E Europa L.  ↓ relegation"
	}
	return th.Info.Render("▌") + th.Muted.Render(" Champions L.  ") +
		th.Below.Render("▌") + th.Muted.Render(" Europa L.  ") +
		th.Negative.Render("▌") + th.Muted.Render(" relegation")
}

// insightGap is the least xPts shortfall worth a line.
const insightGap = 1.0

// insight names the club furthest below its expected points.
func (m *Model) insight() string {
	var best Row
	gap := insightGap
	found := false
	for _, r := range m.visibleRows() {
		x, ok := m.xpts[r.Team.ID]
		if !ok {
			continue
		}
		// On a tie the higher club keeps it.
		if g := x - float64(r.Stats.Points); g >= gap && (!found || g > gap) {
			best, gap, found = r, g, true
		}
	}
	if !found {
		return ""
	}
	return m.deps.Theme.Faint.Render(fmt.Sprintf("%s: %d pts from %.1f xPts · biggest gap in the table",
		best.Team.Name, best.Stats.Points, m.xpts[best.Team.ID]))
}

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n\n")
	if m.showStrip() {
		b.WriteString(m.strip.View(th, m.width) + "\n\n")
	}

	switch {
	case m.err != nil:
		b.WriteString(th.Error.Render("Could not load standings: " + m.err.Error()))
	case m.loading:
		b.WriteString(th.Muted.Render("Loading standings…"))
	case len(m.seasons) == 0:
		b.WriteString(m.emptyState("No seasons cached for this competition."))
	case m.matchesView:
		b.WriteString(m.fixturesView())
	case len(m.rows) == 0:
		season, _ := m.currentSeason()
		b.WriteString(m.emptyState(fmt.Sprintf(
			"No standings cached for %s.", components.SeasonName(season))))
	default:
		b.WriteString(m.table())
		if line := m.insight(); line != "" {
			b.WriteString("\n" + line)
		}
	}

	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme

	name := m.comp.Name
	if name == "" {
		name = m.compID
	}
	// The title already looks selected, so mark the header with an arrow
	// (as the catalog and Team screens do); keep the gutter so it doesn't
	// shift.
	marker := "  "
	if m.HeaderSelected() {
		marker = th.PanelTitle.Render("▸ ")
	}
	title := marker + th.Title.Render(m.deps.Star(models.FavoriteCompetition, m.compID, name))

	var meta []string
	if m.comp.Country != "" {
		meta = append(meta, m.comp.Country)
	}
	if m.comp.Tier > 0 {
		meta = append(meta, "tier "+strconv.Itoa(m.comp.Tier))
	}
	if m.comp.International() {
		meta = append(meta, m.comp.Category)
	}
	if m.comp.Gender != "" {
		meta = append(meta, m.comp.Gender)
	}

	line := title
	if len(meta) > 0 {
		line = lipgloss.JoinHorizontal(lipgloss.Center, title, th.Subtitle.Render("  "+strings.Join(meta, " · ")))
	}

	// Season tabs get the width the freshness note leaves.
	freshness := m.freshnessNote()
	budget := m.width
	if freshness != "" {
		budget -= lipgloss.Width(freshness) + 3
	}
	if seasonBar := m.seasonBar(budget); seasonBar != "" {
		if freshness != "" {
			seasonBar += "   " + freshness
		}
		line += "\n" + seasonBar
	}
	if legend := m.zoneLegend(); legend != "" && !m.matchesView {
		line += "\n" + legend
	}
	if groups := m.groupBar(); groups != "" && !m.matchesView {
		line += "\n" + groups
	}
	return line
}

// freshnessNote reports a failed refresh as a failure, not as age.
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

// seasonBar is shown even for one season, so it's clear which one this is.
// Oldest is leftmost so the left arrow (older) moves left on screen.
func (m *Model) seasonBar(maxWidth int) string {
	th := m.deps.Theme
	n := len(m.seasons)
	if n == 0 {
		return ""
	}

	// m.seasons is newest first; display position d holds season n-1-d.
	tabs := make([]string, n)
	for d := 0; d < n; d++ {
		i := n - 1 - d
		tabs[d] = components.Zone("season:"+strconv.Itoa(i), components.TabLabel(th, components.SeasonName(m.seasons[i]), i == m.seasonIdx))
	}
	return components.WindowTabs(th, tabs, n-1-m.seasonIdx, maxWidth)
}

func (m *Model) table() string {
	th := m.deps.Theme
	rows := make([][]components.Cell, 0, len(m.rows))

	extras := m.extraGroups()
	// A club competition's group tab is renumbered from 1 and has no
	// zones, which go by overall place.
	inGroup := m.Group() != "" && !m.comp.International()
	for i, r := range m.visibleRows() {
		s := r.Stats
		pos, zone := s.Position, m.zoneCell(s.Position)
		if inGroup {
			n := i + 1
			pos, zone = &n, components.Text(" ")
		}

		row := []components.Cell{
			zone,
			components.Text(components.Int(pos)),
			components.Text(components.Dot(th, m.colors[r.Team.ID].PrimaryHex) + " " +
				m.deps.Star(models.FavoriteTeam, r.Team.ID, r.Team.Name)),
			components.Text(strconv.Itoa(s.MatchesPlayed)),
			components.Text(strconv.Itoa(s.Wins)),
			components.Text(strconv.Itoa(s.Draws)),
			components.Text(strconv.Itoa(s.Losses)),
			components.Text(strconv.Itoa(s.GoalsFor)),
			components.Text(strconv.Itoa(s.GoalsAgainst)),
			components.Styled(components.Signed(s.GoalDiff), m.deltaStyle(float64(s.GoalDiff))),
			components.Styled(strconv.Itoa(s.Points), th.Accent.Bold(true)),
		}
		for _, g := range extras {
			row = append(row, g.cells(s)...)
		}
		rows = append(rows, row)
	}

	cols := m.columns()
	for i := range rows {
		rows[i] = components.TrimRow(rows[i], len(cols))
	}

	return components.Table(components.TableOpts{
		Columns: cols,
		Rows:    rows,
		Cursor:  m.cursor,
		MaxRows: components.TableRows(m.tableHeight(), len(rows)),
		Theme:   th,
		Zone:    "row",
	})
}

// deltaStyle: under NO_COLOR the +/- sign carries the meaning.
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

// minStripBody must remain below the score strip for it to be drawn.
const minStripBody = 14

func (m *Model) showStrip() bool {
	return !m.strip.Empty() &&
		m.height-components.Lines(m.header())-1-m.strip.Height()-1 >= minStripBody
}

// tableHeight is measured, not guessed, so the table meets the footer.
func (m *Model) tableHeight() int {
	used := components.Lines(m.header()) + 1
	if m.showStrip() {
		used += m.strip.Height() + 1
	}
	if m.insight() != "" && !m.matchesView {
		used++
	}
	return max(m.height-used, 3)
}

func (m *Model) emptyState(reason string) string {
	th := m.deps.Theme
	command := "footui refresh --competition " + m.compID
	if season, ok := m.currentSeason(); ok && m.comp.International() {
		command += " --season " + season.Label
	}
	// Style lines separately: Lip Gloss pads multi-line renders to the
	// longest line.
	lines := []string{
		th.Empty.Render(reason),
		"",
		th.Muted.Render("Press  ") + th.Accent.Render("r") + th.Muted.Render("  to fetch it now,"),
		th.Muted.Render("or run  ") +
			th.Accent.Render(command) +
			th.Muted.Render("  from a shell."),
	}
	return strings.Join(lines, "\n")
}
