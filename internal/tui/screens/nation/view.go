package nation

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	if !m.strip.Empty() {
		b.WriteString("\n" + m.strip.View(th, m.width) + "\n\n")
	}
	switch {
	case m.err != nil:
		b.WriteString("\n" + th.Error.Render("Could not load matches: "+m.err.Error()))
	case !m.loaded:
		b.WriteString("\n" + th.Muted.Render("Loading matches…"))
	case len(m.years) == 0:
		b.WriteString("\n" + m.emptyState())
	default:
		b.WriteString(m.filterBar())
		b.WriteString("\n\n")
		b.WriteString(m.summaryTable())
		b.WriteString("\n\n")
		if len(m.matches) == 0 {
			b.WriteString(th.Empty.Render("No matches pass this filter."))
		} else {
			b.WriteString(m.matchTable())
		}
	}
	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme
	marker := "  "
	if m.cursor == headerRow && m.focus == paneMatches {
		marker = th.PanelTitle.Render("▸ ")
	}
	title := marker + th.Title.Render(m.deps.Star(models.FavoriteTeam, m.team.ID, m.team.Name))
	meta := "national team"
	if m.team.Gender != "" {
		meta += " · " + m.team.Gender
	}
	line := lipgloss.JoinHorizontal(lipgloss.Center, title, th.Subtitle.Render("  "+meta))
	note := ""
	switch {
	case m.refreshing:
		note = th.Status.Render("refreshing now…")
	case m.queued:
		note = th.Status.Render("queued for refresh")
	}
	if bar := m.yearBar(); bar != "" {
		line += "\n  " + bar
		if note != "" {
			line += "   " + note
		}
	} else if note != "" {
		line += "   " + note
	}
	return line
}

// yearBar is "‹ 2025  [2026]  All years ›": each year with matches,
// oldest on the left, then all of them together.
func (m *Model) yearBar() string {
	th := m.deps.Theme
	if len(m.years) == 0 {
		return ""
	}
	var parts []string
	for i := 0; i <= len(m.years); i++ {
		label := "All years"
		if i < len(m.years) {
			label = strconv.Itoa(m.years[i])
		}
		if i == m.yearIdx {
			parts = append(parts, th.Accent.Bold(true).Render("["+label+"]"))
		} else {
			parts = append(parts, th.Muted.Render(" "+label+" "))
		}
	}
	return th.Muted.Render("‹ ") + strings.Join(parts, " ") + th.Muted.Render(" ›")
}

// filterBar is "Filter: [All] Competitive Tournaments │ ✓WC ✓WCQ ☐FRI".
// Presets are bracketed when in effect; each competition shows whether
// the filter keeps it. With the bar focused, its cursor is underlined.
func (m *Model) filterBar() string {
	th := m.deps.Theme
	focused := m.focus == paneFilter
	stop := func(i int, text string, on bool) string {
		style := th.Muted
		if on {
			style = th.Accent.Bold(true)
		}
		if focused && i == m.filterIdx {
			style = style.Underline(true).Reverse(true)
		}
		return style.Render(text)
	}
	var parts []string
	for i, p := range presets {
		on := m.filter.Preset == p.key || (p.key == models.PresetAll && m.filter.IsAll())
		text := " " + p.label + " "
		if on {
			text = "[" + p.label + "]"
		}
		parts = append(parts, stop(i, text, on))
	}
	parts = append(parts, th.Muted.Render("│"))
	for i, c := range m.comps {
		box := "☐"
		keep := m.filter.Keeps(c)
		if keep {
			box = "✓"
		}
		parts = append(parts, stop(len(presets)+i, box+c.Abbrev(), keep))
	}
	label := "  Filter: "
	if focused {
		label = th.PanelTitle.Render("▸ Filter: ")
	} else {
		label = th.Muted.Render(label)
	}
	return components.Truncate(label+strings.Join(parts, " "), max(m.width, 20))
}

func (m *Model) summaryTable() string {
	th := m.deps.Theme
	s := m.summary
	avg := func(v *float64, suffix string, decimals int) string {
		if v == nil {
			return components.Missing
		}
		return strconv.FormatFloat(*v, 'f', decimals, 64) + suffix
	}
	cols := []components.Column{
		{Title: "P", Width: 3, Right: true}, {Title: "W", Width: 3, Right: true},
		{Title: "D", Width: 3, Right: true}, {Title: "L", Width: 3, Right: true},
		{Title: "GF", Width: 4, Right: true}, {Title: "GA", Width: 4, Right: true},
		{Title: "Poss", Width: 5, Right: true}, {Title: "Sh", Width: 5, Right: true},
		{Title: "SoT", Width: 5, Right: true}, {Title: "Form", Width: 6, Right: true},
	}
	row := []components.Cell{
		components.Text(strconv.Itoa(s.Played)), components.Text(strconv.Itoa(s.Won)),
		components.Text(strconv.Itoa(s.Drawn)), components.Text(strconv.Itoa(s.Lost)),
		components.Text(strconv.Itoa(s.GoalsFor)), components.Text(strconv.Itoa(s.GoalsAgainst)),
		components.Text(avg(s.Possession, "%", 0)), components.Text(avg(s.Shots, "", 1)),
		components.Text(avg(s.ShotsOnTarget, "", 1)), components.Styled(s.Form, th.Accent.Bold(true)),
	}
	return components.Table(components.TableOpts{Columns: cols, Rows: [][]components.Cell{row}, Cursor: -1, Theme: th})
}

// matchTable lists the matches newest first. The expanded match's box
// score is drawn as extra rows right under it, so the cursor's row index
// in the table is shifted past them.
func (m *Model) matchTable() string {
	th := m.deps.Theme
	roundWidth := max(m.width-(1+1+10+1+6+1+3+1+22+1+15+1)-4, 8)
	cols := append(components.MatchColumns(22), components.Column{Title: "Round", Width: roundWidth})
	var rows [][]components.Cell
	cursor := -1
	for i, mt := range m.matches {
		if i == m.cursor {
			cursor = len(rows)
		}
		rows = append(rows, m.matchRow(mt))
		if mt.ID == m.expanded {
			rows = append(rows, m.boxScore(mt)...)
		}
	}
	return components.Table(components.TableOpts{
		Columns: cols, Rows: rows, Cursor: cursor, MaxRows: m.tableHeight(), Theme: th,
	})
}

func (m *Model) matchRow(mt models.Match) []components.Cell {
	return components.MatchCells(m.deps.Theme, mt, m.team.ID, m.competition(mt.CompetitionID),
		m.teamName(mt.Opponent(m.team.ID)), mt.Round)
}

// boxScore is the two sides' numbers from one match, the nation's first.
func (m *Model) boxScore(mt models.Match) [][]components.Cell {
	return components.BoxScore(m.deps.Theme, mt, m.team.ID, 4)
}

func (m *Model) tableHeight() int {
	chrome := 12
	if !m.strip.Empty() {
		chrome += m.strip.Height() + 2 // and the blank lines either side
	}
	return max(m.height-chrome, 5)
}

func (m *Model) emptyState() string {
	th := m.deps.Theme
	lines := []string{
		th.Empty.Render("No matches cached for " + m.team.Name + "."),
		"",
		th.Muted.Render("Press  ") + th.Accent.Render("r") +
			th.Muted.Render("  to fetch this year's and last year's — shared with every nation, so the next costs nothing —"),
		th.Muted.Render("or run  ") + th.Accent.Render("footui refresh --team "+teamArg(m.team)) + th.Muted.Render("  from a shell."),
	}
	return strings.Join(lines, "\n")
}

func teamArg(t models.Team) string {
	arg := strings.ToLower(t.Name)
	if t.Gender == "women" {
		arg += " women"
	}
	if strings.Contains(arg, " ") {
		return `"` + arg + `"`
	}
	return arg
}
