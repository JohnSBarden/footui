package team

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Layout: parts are measured as drawn and the tables get what's left;
// don't hard-code the heights above them.
const (
	// minTablesLines is reserved for the tables below the stat cards.
	minTablesLines = 9
	// minStripBody must remain below the score strip for it to be drawn.
	minStripBody = 14
	// minPaneRows: below this per table, only the focused one is drawn.
	minPaneRows = 3
	paneGap     = 4
	// squadMinWidth fits the squad's base columns plus xG side by side.
	squadMinWidth = 80
)

// View implements core.Screen.
func (m *Model) View() string {
	th := m.theme
	parts := []string{m.header()}
	rest := func() int {
		return m.height - components.Lines(strings.Join(parts, "\n\n")) - 1
	}

	switch {
	case m.err != nil:
		return strings.Join(append(parts, th.Error.Render("Could not load this team: "+m.err.Error())), "\n\n")
	case m.loading:
		return strings.Join(append(parts, th.Muted.Render("Loading…")), "\n\n")
	case len(m.competitions) == 0 && len(m.recent) == 0 && !m.fetching && !m.queued:
		if !m.strip.Empty() {
			parts = append(parts, m.strip.View(th, m.width))
		}
		return strings.Join(append(parts, m.emptyState()), "\n\n")
	}

	if m.showStrip() {
		parts = append(parts, m.strip.View(th, m.width))
	}
	parts = append(parts, m.recentStrip())
	if len(m.competitions) == 0 {
		// Known only from its games (e.g. a cup opponent): no tables.
		return strings.Join(parts, "\n\n")
	}

	if stats := m.stats(); len(stats) > 0 {
		room := rest()
		budget := min(room/3, room-minTablesLines)
		parts = append(parts, components.StatPanel(th, stats, m.width, budget))
	}

	parts = append(parts, m.tables(rest()))
	return strings.Join(parts, "\n\n")
}

func (m *Model) showStrip() bool {
	if m.strip.Empty() {
		return false
	}
	above := components.Lines(m.header()) + 1 + m.strip.Height() + 1 +
		components.Lines(m.recentStrip()) + 1
	return m.height-above >= minStripBody
}

func (m *Model) tables(lines int) string {
	seasonsWidth := components.ColumnsWidth(m.seasonColumns(m.width))
	if squadWidth := m.width - seasonsWidth - paneGap; squadWidth >= squadMinWidth {
		left := m.seasonsTable(components.TableRows(lines-1, len(m.CurrentRows())), m.width)
		if panel := m.honoursPanel(); panel != "" && components.Lines(left)+1+components.Lines(panel) <= lines {
			left += "\n\n" + panel
		}
		right := m.squadTable(components.TableRows(lines-1, len(m.roster)), squadWidth)
		return components.SideBySide(paneGap, []int{seasonsWidth, squadWidth}, left, right)
	}

	seasons, squad := len(m.CurrentRows()), max(len(m.roster), 1)
	need := func(rows int) int { return 1 + components.TableLines(rows, rows) }
	if need(min(seasons, minPaneRows))+1+need(min(squad, minPaneRows)) > lines {
		return m.focusedPane(lines)
	}
	// Seasons get up to a third of the room; the squad gets the rest.
	seasonLines := min(need(seasons), max(lines/3, need(minPaneRows)))
	top := m.seasonsTable(components.TableRows(seasonLines-1, seasons), m.width)
	squadLines := lines - components.Lines(top) - 1
	out := top + "\n\n" + m.squadTable(components.TableRows(squadLines-1, len(m.roster)), m.width)
	if panel := m.honoursPanel(); panel != "" && components.Lines(out)+1+components.Lines(panel) <= lines {
		out += "\n\n" + panel
	}
	return out
}

func (m *Model) focusedPane(lines int) string {
	th := m.theme
	label := func(text string, on bool) string {
		if on {
			return th.PanelTitle.Render("▸ " + text)
		}
		return th.Muted.Render(text)
	}
	title := label("Seasons", m.focus == paneSeasons) + "   " +
		label("Squad"+m.squadSeason(), m.focus == paneSquad) + th.Muted.Render("   tab switch")
	var body string
	if m.focus == paneSquad {
		body = m.squadBody(components.TableRows(lines-1, len(m.roster)), m.width)
	} else {
		body = m.seasonsBody(components.TableRows(lines-1, len(m.CurrentRows())), m.width)
	}
	return title + "\n" + body
}

func (m *Model) header() string {
	th := m.theme

	var meta []string
	for _, part := range []string{m.team.Country, m.team.Gender, m.team.Venue, m.freshnessText()} {
		if part != "" {
			meta = append(meta, part)
		}
	}
	var colors models.TeamColors
	if m.colors != nil {
		colors = *m.colors
	}
	fill, rule := styles.HeroStyles(th, colors.PrimaryHex, colors.SecondaryHex, m.deps.ThemeMode())
	hero := components.Hero(components.HeroOpts{
		Fill: fill, Rule: rule, Width: m.width, NoColor: th.NoColor,
		Title: m.deps.Star(models.FavoriteTeam, m.team.ID, m.team.Name),
		Meta:  strings.Join(meta, " · "),
		Right: m.heroRight(),
	})

	if len(m.competitions) == 0 {
		return hero
	}

	tabs := make([]components.Tab, 0, len(m.competitions))
	for _, c := range m.competitions {
		tabs = append(tabs, components.Tab{
			Label: c.Name,
			Badge: "(" + strconv.Itoa(len(m.history[c.ID])) + ")",
		})
	}
	return hero + "\n" + components.Tabs(th, tabs, m.compIdx)
}

func (m *Model) heroRight() []string {
	row, ok := m.CurrentSeasonRow()
	if !ok || row.Stats.Position == nil {
		return nil
	}
	pos := *row.Stats.Position
	digits := components.BlockDigits(strconv.Itoa(pos))
	side := []string{
		components.SeasonName(row.Season),
		components.OrdinalSuffix(pos) + " · " + strconv.Itoa(row.Stats.Points) + " pts",
		m.finishes(),
	}
	width := 0
	for _, s := range side {
		width = max(width, lipgloss.Width(s))
	}
	out := make([]string, len(side))
	for i, s := range side {
		out[i] = digits[i] + "  " + s + strings.Repeat(" ", width-lipgloss.Width(s))
	}
	return out
}

const finishesShown = 3

// finishes is oldest first: "15 ▸ 3 ▸ 13".
func (m *Model) finishes() string {
	rows := m.CurrentRows() // newest first
	var pos []string
	for i := min(len(rows), finishesShown) - 1; i >= 0; i-- {
		if p := rows[i].Stats.Position; p != nil {
			pos = append(pos, strconv.Itoa(*p))
		}
	}
	if len(pos) < 2 {
		return ""
	}
	return "finished " + strings.Join(pos, " ▸ ")
}

// freshnessText reports a failed refresh as a failure, not as age.
func (m *Model) freshnessText() string {
	if m.lastSync == nil {
		return ""
	}
	ago := components.Freshness(m.lastSync.FinishedAt, time.Now())
	if ago == "" {
		return ""
	}
	if !m.lastSync.Succeeded() {
		return "refresh failed " + ago
	}
	return "updated " + ago
}

// stats are ordered most telling first; a short terminal keeps the first.
func (m *Model) stats() []components.Stat {
	row, ok := m.CurrentSeasonRow()
	if !ok {
		return nil
	}
	s := row.Stats

	gdStyle := m.deltaStyle(float64(s.GoalDiff))
	stats := []components.Stat{
		{Label: "Position", Value: components.Int(s.Position), Hint: components.SeasonName(row.Season)},
		{Label: "Points", Value: strconv.Itoa(s.Points), Hint: strconv.Itoa(s.MatchesPlayed) + " played"},
		{Label: "Record", Value: strconv.Itoa(s.Wins) + "-" + strconv.Itoa(s.Draws) + "-" + strconv.Itoa(s.Losses), Hint: "W-D-L"},
		{Label: "Goals", Value: strconv.Itoa(s.GoalsFor) + ":" + strconv.Itoa(s.GoalsAgainst), Hint: components.Signed(s.GoalDiff), Style: &gdStyle},
	}

	if s.XG != nil {
		delta := float64(s.GoalsFor) - *s.XG
		style := m.deltaStyle(delta)
		stats = append(stats, components.Stat{
			Label: "xG", Value: components.Float(s.XG, 1),
			Hint:  verdict(-delta, "unlucky", "clinical"),
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

	return append(stats, m.extraCards(s)...)
}

// extraCards omits a card entirely when its stat is absent.
func (m *Model) extraCards(s models.TeamSeasonStats) []components.Stat {
	var out []components.Stat

	if xpts, ok := m.extras[models.StatExpectedPoints]; ok {
		delta := float64(s.Points) - xpts
		style := m.deltaStyle(delta)
		out = append(out, components.Stat{
			Label: "xPts", Value: strconv.FormatFloat(xpts, 'f', 1, 64),
			Hint:  verdict(-delta, "owed", "lucky"),
			Style: &style,
		})
	}

	if ppda, ok := m.extras[models.StatPPDA]; ok {
		// PPDA is inverted: lower means a harder press.
		out = append(out, components.Stat{
			Label: "PPDA", Value: strconv.FormatFloat(ppda, 'f', 1, 64),
			Hint: "lower = press",
		})
	}

	if npxg, ok := m.extras[models.StatNPxG]; ok {
		out = append(out, components.Stat{
			Label: "npxG", Value: strconv.FormatFloat(npxg, 'f', 1, 64),
			Hint: "excl. pens",
		})
	}

	if deep, ok := m.extras[models.StatDeep]; ok {
		hint := ""
		if allowed, ok := m.extras[models.StatDeepAllowed]; ok {
			hint = strconv.FormatFloat(allowed, 'f', 0, 64) + " allowed"
		}
		out = append(out, components.Stat{
			Label: "Deep", Value: strconv.FormatFloat(deep, 'f', 0, 64),
			Hint: hint,
		})
	}

	return out
}

// verdict: gap is expected minus actual; above labels gap > 0, below < 0.
func verdict(gap float64, above, below string) string {
	switch {
	case gap > 0.5:
		return "▲ " + strconv.FormatFloat(gap, 'f', 1, 64) + " " + above
	case gap < -0.5:
		return "▼ " + strconv.FormatFloat(-gap, 'f', 1, 64) + " " + below
	}
	return "on par"
}

func (m *Model) seasonsTable(rows, width int) string {
	return m.paneTitle("Seasons", m.focus == paneSeasons) + "\n" + m.seasonsBody(rows, width)
}

func (m *Model) seasonsBody(maxRows, width int) string {
	th := m.theme
	rows := m.CurrentRows()
	if len(rows) == 0 {
		return th.Empty.Render("No seasons cached for this competition.")
	}

	cells := make([][]components.Cell, 0, len(rows))
	for _, r := range rows {
		s := r.Stats
		cells = append(cells, []components.Cell{
			components.Text(components.SeasonName(r.Season)),
			components.Text(components.Int(s.Position)),
			components.Text(strconv.Itoa(s.MatchesPlayed)),
			components.Text(strconv.Itoa(s.Wins)),
			components.Text(strconv.Itoa(s.Draws)),
			components.Text(strconv.Itoa(s.Losses)),
			components.Text(strconv.Itoa(s.GoalsFor)),
			components.Text(strconv.Itoa(s.GoalsAgainst)),
			components.Styled(components.Signed(s.GoalDiff), m.deltaStyle(float64(s.GoalDiff))),
			components.Styled(strconv.Itoa(s.Points), th.Accent.Bold(true)),
		})
	}
	for _, g := range m.seasonGroups() {
		for i, r := range rows {
			cells[i] = append(cells[i], g.cells(r.Stats)...)
		}
	}

	cursor := -1
	if m.focus == paneSeasons {
		cursor = m.seasonIdx
	}

	cols := m.seasonColumns(width)
	for i := range cells {
		cells[i] = components.TrimRow(cells[i], len(cols))
	}

	return components.Table(components.TableOpts{
		Columns: cols,
		Rows:    cells,
		Cursor:  cursor,
		MaxRows: maxRows,
		Theme:   th,
		Zone:    "season",
	})
}

func (m *Model) seasonColumns(width int) []components.Column {
	base := []components.Column{
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
	}

	var groups [][]components.Column
	for _, g := range m.seasonGroups() {
		groups = append(groups, g.columns)
	}
	return components.FitColumns(width, base, 0, groups)
}

// extraGroup is optional columns shown or dropped together.
type extraGroup[T any] struct {
	columns []components.Column
	cells   func(T) []components.Cell
}

// seasonGroups matches the League screen: xG if any, else possession and
// shots. Columns with no values are never shown.
func (m *Model) seasonGroups() []extraGroup[models.TeamSeasonStats] {
	rows := m.CurrentRows()
	has := func(f func(models.TeamSeasonStats) bool) bool {
		return slices.ContainsFunc(rows, func(r SeasonRow) bool { return f(r.Stats) })
	}
	if has(func(s models.TeamSeasonStats) bool { return s.XG != nil }) {
		return []extraGroup[models.TeamSeasonStats]{{
			columns: []components.Column{
				{Title: "xG", Width: 6, Right: true},
				{Title: "xGA", Width: 6, Right: true},
			},
			cells: func(s models.TeamSeasonStats) []components.Cell {
				return []components.Cell{
					components.Text(components.Float(s.XG, 1)),
					components.Text(components.Float(s.XGA, 1)),
				}
			},
		}}
	}
	var groups []extraGroup[models.TeamSeasonStats]
	if has(func(s models.TeamSeasonStats) bool { return s.PossessionAvg != nil }) {
		groups = append(groups, extraGroup[models.TeamSeasonStats]{
			columns: []components.Column{{Title: "Poss", Width: 6, Right: true}},
			cells: func(s models.TeamSeasonStats) []components.Cell {
				if s.PossessionAvg == nil {
					return []components.Cell{components.Text(components.Missing)}
				}
				return []components.Cell{components.Text(strconv.FormatFloat(*s.PossessionAvg, 'f', 1, 64) + "%")}
			},
		})
	}
	if has(func(s models.TeamSeasonStats) bool { return s.Shots != nil }) {
		groups = append(groups, extraGroup[models.TeamSeasonStats]{
			columns: []components.Column{
				{Title: "Sh", Width: 4, Right: true},
				{Title: "SoT", Width: 4, Right: true},
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

func (m *Model) squadSeason() string {
	if row, ok := m.CurrentSeasonRow(); ok {
		return " — " + components.SeasonName(row.Season)
	}
	return ""
}

func (m *Model) squadTable(rows, width int) string {
	return m.paneTitle("Squad"+m.squadSeason(), m.focus == paneSquad) + "\n" + m.squadBody(rows, width)
}

func (m *Model) squadBody(maxRows, width int) string {
	th := m.theme
	switch {
	case m.rosterLoading:
		return th.Muted.Render("Loading squad…")
	case len(m.roster) == 0:
		return th.Empty.Render("No squad cached for this season.")
	}

	cells := make([][]components.Cell, 0, len(m.roster))
	for _, r := range m.roster {
		s := r.Stats
		row := []components.Cell{
			components.Text(r.Player.Name),
			components.Text(r.Player.Position),
			components.Text(strconv.Itoa(s.Appearances)),
		}
		if m.squadHasMinutes() {
			row = append(row, components.Text(strconv.Itoa(s.Minutes)))
		}
		row = append(row,
			components.Styled(strconv.Itoa(s.Goals), components.ToneOr(th, s.Goals, s.XG, th.Accent.Bold(true))),
			components.Styled(strconv.Itoa(s.Assists), components.ToneOr(th, s.Assists, s.XA, th.TableCell)),
			components.Text(th.Accent.Render(strings.Repeat("▮", min(s.Goals, squadBarMax)))+
				th.Info.Render(strings.Repeat("▯", min(s.Assists, squadBarMax)))),
		)
		for _, g := range m.squadGroups() {
			row = append(row, g.cells(s)...)
		}
		cells = append(cells, row)
	}

	cursor := -1
	if m.focus == paneSquad {
		cursor = m.cursorSquad
	}

	cols := m.squadColumns(width)
	for i := range cells {
		cells[i] = components.TrimRow(cells[i], len(cols))
	}

	return components.Table(components.TableOpts{
		Columns: cols,
		Rows:    cells,
		Cursor:  cursor,
		MaxRows: maxRows,
		Theme:   th,
		Zone:    "squad",
	})
}

// squadBarMax caps each half of the G·A bar so the column stays narrow.
const squadBarMax = 5

// playerColWidth never shrinks — see components.FitColumns.
const playerColWidth = 22

func (m *Model) squadColumns(width int) []components.Column {
	base := []components.Column{
		{Title: "Player", Width: playerColWidth},
		{Title: "Pos", Width: 6},
		{Title: "Apps", Width: 5, Right: true},
	}
	if m.squadHasMinutes() {
		base = append(base, components.Column{Title: "Mins", Width: 5, Right: true})
	}
	base = append(base,
		components.Column{Title: "G", Width: 4, Right: true},
		components.Column{Title: "A", Width: 4, Right: true},
		components.Column{Title: "G·A", Width: 2 * squadBarMax},
	)

	var groups [][]components.Column
	for _, g := range m.squadGroups() {
		groups = append(groups, g.columns)
	}
	return components.FitColumns(width, base, 0, groups)
}

// squadHasMinutes: some providers have no minutes at all.
func (m *Model) squadHasMinutes() bool {
	return slices.ContainsFunc(m.roster, func(r RosterRow) bool { return r.Stats.Minutes > 0 })
}

// squadGroups are dropped in order (xG, then shots/KP) as width shrinks.
func (m *Model) squadGroups() []extraGroup[models.PlayerSeasonStats] {
	has := func(f func(models.PlayerSeasonStats) bool) bool {
		return slices.ContainsFunc(m.roster, func(r RosterRow) bool { return f(r.Stats) })
	}
	var groups []extraGroup[models.PlayerSeasonStats]
	if has(func(s models.PlayerSeasonStats) bool { return s.XG != nil || s.XA != nil }) {
		groups = append(groups, extraGroup[models.PlayerSeasonStats]{
			columns: []components.Column{
				{Title: "xG", Width: 6, Right: true},
				{Title: "xA", Width: 6, Right: true},
			},
			cells: func(s models.PlayerSeasonStats) []components.Cell {
				return []components.Cell{
					components.Text(components.Float(s.XG, 1)),
					components.Text(components.Float(s.XA, 1)),
				}
			},
		})
	}
	if has(func(s models.PlayerSeasonStats) bool { return s.Shots != nil || s.KeyPasses != nil }) {
		groups = append(groups, extraGroup[models.PlayerSeasonStats]{
			columns: []components.Column{
				{Title: "Sh", Width: 4, Right: true},
				{Title: "KP", Width: 4, Right: true},
			},
			cells: func(s models.PlayerSeasonStats) []components.Cell {
				return []components.Cell{
					components.Text(components.Int(s.Shots)),
					components.Text(components.Int(s.KeyPasses)),
				}
			},
		})
	}
	return groups
}

// paneTitle's arrow keeps focus visible without colour.
func (m *Model) paneTitle(label string, focused bool) string {
	th := m.theme
	if focused {
		return th.PanelTitle.Render("▸ " + label)
	}
	return th.Muted.Render("  " + label)
}

func (m *Model) deltaStyle(v float64) lipgloss.Style {
	switch {
	case v > 0:
		return m.theme.Positive
	case v < 0:
		return m.theme.Negative
	default:
		return m.theme.Muted
	}
}

func (m *Model) emptyState() string {
	th := m.theme
	// Style lines separately: Lip Gloss pads multi-line renders to the
	// longest line.
	lines := []string{th.Empty.Render("No cached seasons for " + m.team.Name + "."), ""}
	if compID, ok := m.RefreshTarget(); ok {
		lines = append(lines,
			th.Muted.Render("Press  ")+th.Accent.Render("r")+th.Muted.Render("  to fetch them now,"),
			th.Muted.Render("or run  ")+
				th.Accent.Render("footui refresh --competition "+compID)+
				th.Muted.Render("  from a shell."),
		)
	} else {
		// No competition to refresh from here; offer only the CLI form.
		lines = append(lines,
			th.Muted.Render("Run  ")+
				th.Accent.Render("footui refresh")+
				th.Muted.Render("  for a competition this team plays in."),
		)
	}
	return strings.Join(lines, "\n")
}
