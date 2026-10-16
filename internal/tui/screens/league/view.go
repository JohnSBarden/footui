package league

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// Column widths. These are fixed rather than proportional: standings are
// read by scanning down a column, which only works if the digits line up.
const (
	posWidth   = 3
	smallWidth = 3 // MP, W, D, L
	goalWidth  = 4 // GF, GA, GD, Pts
	xgWidth    = 6 // xG, xGA, xGD
	// wantTeamCol is wide enough for the longest club names footui covers
	// ("Wolverhampton Wanderers", "Borussia Mönchengladbach") to stay
	// distinguishable from each other. It never shrinks; see
	// components.FitColumns for why.
	wantTeamCol = 24
)

func (m *Model) columns() []components.Column {
	base := []components.Column{
		{Title: "#", Width: posWidth, Right: true},
		{Title: "Team", Width: wantTeamCol},
		{Title: "MP", Width: smallWidth, Right: true},
		{Title: "W", Width: smallWidth, Right: true},
		{Title: "D", Width: smallWidth, Right: true},
		{Title: "L", Width: smallWidth, Right: true},
		{Title: "GF", Width: goalWidth, Right: true},
		{Title: "GA", Width: goalWidth, Right: true},
		{Title: "GD", Width: goalWidth, Right: true},
		{Title: "Pts", Width: goalWidth, Right: true},
	}

	// The team column is already in base, so nothing extra is reserved.
	var groups [][]components.Column
	for _, g := range m.extraGroups() {
		groups = append(groups, g.columns)
	}
	return components.FitColumns(m.width, base, 0, groups)
}

// extraGroup is a set of optional columns that are shown or dropped
// together, with the cells each row contributes to them.
type extraGroup struct {
	columns []components.Column
	cells   func(s models.TeamSeasonStats) []components.Cell
}

// extraGroups picks the optional columns this competition's data can
// fill, in the order they're given up on a narrow terminal. Competitions
// from understat have expected goals; those from ESPN have none, but may
// have possession and shots instead. A column nobody in the table has a
// value for is never shown: a column of dashes is just noise.
func (m *Model) extraGroups() []extraGroup {
	has := func(f func(models.TeamSeasonStats) bool) bool {
		for _, r := range m.rows {
			if f(r.Stats) {
				return true
			}
		}
		return false
	}
	var groups []extraGroup
	if has(func(s models.TeamSeasonStats) bool { return s.XG != nil }) {
		// xG and xGA go together: attacking output means little without
		// what the team conceded. xGD is derived from the pair.
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
		return groups
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
	return groups
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
	case m.matchesView:
		b.WriteString(m.fixturesView())
	case len(m.rows) == 0:
		season, _ := m.currentSeason()
		b.WriteString(m.emptyState(fmt.Sprintf(
			"No standings cached for %s.", components.SeasonName(season))))
	default:
		b.WriteString(m.table())
	}

	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme

	name := m.comp.Name
	if name == "" {
		name = m.compID
	}
	// The header is one more stop for the cursor above the first club,
	// marked the way the catalog and Team screens mark the focused pane.
	// The title is already drawn like a selected row, so a highlight
	// alone wouldn't show; the gutter is kept when it's not selected so
	// the title doesn't shift as the cursor moves.
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

	// The season tabs share their line with the freshness note, so they
	// get whatever width the note leaves.
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
	if groups := m.groupBar(); groups != "" && !m.matchesView {
		line += "\n" + groups
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
//
// Seasons read left to right in time, oldest first, so the left arrow
// (an older season) moves left on screen. Every cached season is offered;
// when they don't all fit, the bar shows a window around the selected one
// with ‹ and › marking seasons off either end.
func (m *Model) seasonBar(maxWidth int) string {
	th := m.deps.Theme
	n := len(m.seasons)
	if n == 0 {
		return ""
	}

	// m.seasons is most recent first; display position d holds season
	// n-1-d, so the selected season sits at n-1-seasonIdx.
	tabs := make([]string, n)
	for d := 0; d < n; d++ {
		i := n - 1 - d
		label := components.SeasonName(m.seasons[i])
		if i == m.seasonIdx {
			tabs[d] = th.Accent.Bold(true).Render("[" + label + "]")
		} else {
			tabs[d] = th.Muted.Render(" " + label + " ")
		}
	}
	sel := n - 1 - m.seasonIdx

	// Grow a window outward from the selection while it fits, preferring
	// newer seasons, and leaving room for the ‹ › markers.
	const marker = 2
	width := func(lo, hi int) int {
		w := 0
		for d := lo; d <= hi; d++ {
			w += lipgloss.Width(tabs[d]) + 1
		}
		if lo > 0 {
			w += marker
		}
		if hi < n-1 {
			w += marker
		}
		return w - 1
	}
	lo, hi := sel, sel
	for grew := true; grew; {
		grew = false
		if hi < n-1 && width(lo, hi+1) <= maxWidth {
			hi++
			grew = true
		}
		if lo > 0 && width(lo-1, hi) <= maxWidth {
			lo--
			grew = true
		}
	}

	var b strings.Builder
	if lo > 0 {
		b.WriteString(th.Muted.Render("‹ "))
	}
	b.WriteString(strings.Join(tabs[lo:hi+1], " "))
	if hi < n-1 {
		b.WriteString(th.Muted.Render(" ›"))
	}
	return b.String()
}

func (m *Model) table() string {
	th := m.deps.Theme
	rows := make([][]components.Cell, 0, len(m.rows))

	extras := m.extraGroups()
	for _, r := range m.visibleRows() {
		s := r.Stats

		row := []components.Cell{
			components.Text(components.Int(s.Position)),
			components.Text(m.deps.Star(models.FavoriteTeam, r.Team.ID, r.Team.Name)),
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
	chrome := 9
	if len(m.groups) > 1 {
		chrome++ // the group bar
	}
	h := m.height - chrome
	if h < 5 {
		return 5
	}
	return h
}

func (m *Model) emptyState(reason string) string {
	th := m.deps.Theme
	command := "footui refresh --competition " + m.compID
	if season, ok := m.currentSeason(); ok && m.comp.International() {
		command += " --season " + season.Label
	}
	// Each line is styled separately and joined afterwards: a newline
	// inside a single Render call makes Lip Gloss pad every line to the
	// width of the longest, which leaves the shorter one visibly indented.
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
