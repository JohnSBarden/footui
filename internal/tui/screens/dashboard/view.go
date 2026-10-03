package dashboard

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// Card geometry. A card is cardWidth columns wide including its border,
// and holds cardLines lines of content: a name, a context line, five
// table rows and a freshness line for a league, which is the tallest.
const (
	cardWidth = 38
	cardGap   = 1
	cardLines = 8
	// headerLines is the dashboard's own title and the blank line under it.
	headerLines = 2
)

// now is the clock freshness is measured against; tests pin it.
var now = time.Now

// columns is how many cards fit side by side; one on a narrow terminal.
func (m *Model) columns() int {
	cols := (m.width + cardGap) / (cardWidth + cardGap)
	if cols < 1 {
		return 1
	}
	return cols
}

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder
	b.WriteString(th.Title.Render("footy"))
	b.WriteString(th.Subtitle.Render("  your favourites"))
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(th.Error.Render("Couldn't load favourites: " + m.err.Error()))
	case m.loading:
		b.WriteString(th.Muted.Render("Loading…"))
	case len(m.cards) == 0:
		lines := []string{
			th.Empty.Render("No favourites yet."),
			"",
			th.Muted.Render("Press  ") + th.Accent.Render("/") + th.Muted.Render("  to search, open a league, club or player,"),
			th.Muted.Render("and press  ") + th.Accent.Render("f") + th.Muted.Render("  to star it here."),
		}
		b.WriteString(strings.Join(lines, "\n"))
	default:
		b.WriteString(m.cardsView())
	}
	return b.String()
}

// block is one vertical slice of the card area: a group heading or a row
// of cards.
type block struct {
	text  string
	lines int
	// row is the grid row this block draws, -1 for a heading.
	row int
}

// cardsView lays the cards out in groups, scrolled so the selected card's
// row is on screen.
func (m *Model) cardsView() string {
	th := m.deps.Theme
	rows := m.grid()
	selRow, _ := position(rows, m.cursor)

	var blocks []block
	for r, row := range rows {
		if r == 0 || m.cards[row[0]].Kind != m.cards[rows[r-1][0]].Kind {
			if r > 0 {
				blocks = append(blocks, block{text: "", lines: 1, row: -1})
			}
			blocks = append(blocks, block{text: th.PanelTitle.Render(groupName(m.cards[row[0]].Kind)), lines: 1, row: -1})
		}
		rendered := make([]string, 0, len(row))
		for c, i := range row {
			if c > 0 {
				rendered = append(rendered, strings.Repeat(" ", cardGap))
			}
			rendered = append(rendered, m.renderCard(m.cards[i], i == m.cursor))
		}
		text := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
		blocks = append(blocks, block{text: text, lines: lipgloss.Height(text), row: r})
	}

	// Scroll: the first block shown is the earliest one that still lets
	// the selected row fit, so moving down scrolls only once it must.
	budget := m.height - headerLines
	last := 0
	for i, bl := range blocks {
		if bl.row == selRow {
			last = i
		}
	}
	first := 0
	for first < last && linesBetween(blocks, first, last) > budget {
		first++
	}
	var out []string
	used := 0
	for _, bl := range blocks[first:] {
		if used+bl.lines > budget && used > 0 {
			break
		}
		out = append(out, bl.text)
		used += bl.lines
	}
	return strings.Join(out, "\n")
}

func linesBetween(blocks []block, from, to int) int {
	n := 0
	for _, bl := range blocks[from : to+1] {
		n += bl.lines
	}
	return n
}

func groupName(k models.FavoriteKind) string {
	switch k {
	case models.FavoriteCompetition:
		return "Leagues"
	case models.FavoriteTeam:
		return "Clubs"
	default:
		return "Players"
	}
}

// cardInner is the usable width inside a card's border and padding.
func (m *Model) cardInner() int {
	w := cardWidth
	if m.columns() == 1 && m.width < cardWidth {
		w = m.width
	}
	return w - 4
}

// renderCard draws one card. The selected card's border is drawn in the
// accent color and, so it still stands out without color, heavier.
func (m *Model) renderCard(c Card, selected bool) string {
	th := m.deps.Theme
	inner := m.cardInner()
	var lines []string
	switch {
	case c.Missing:
		lines = m.missingLines(c, inner)
	case c.Kind == models.FavoriteCompetition:
		lines = m.leagueLines(c, inner)
	case c.Kind == models.FavoriteTeam:
		lines = m.teamLines(c, inner)
	default:
		lines = m.playerLines(c, inner)
	}
	for len(lines) < cardLines {
		lines = append(lines, "")
	}

	style := th.Panel.Width(inner + 2).Height(cardLines)
	if selected {
		style = style.Border(lipgloss.ThickBorder()).BorderForeground(th.Palette.Accent)
		if th.NoColor {
			style = style.UnsetBorderForeground()
		}
	}
	return style.Render(strings.Join(lines[:cardLines], "\n"))
}

func (m *Model) heading(c Card, inner int) string {
	return m.deps.Theme.PanelTitle.Render(components.Truncate("★ "+c.Name(), inner))
}

func (m *Model) missingLines(c Card, inner int) []string {
	th := m.deps.Theme
	return []string{
		m.heading(c, inner),
		th.Muted.Render(string(c.Kind)),
		"",
		th.Empty.Render(components.Truncate("Not cached.", inner)),
		th.Muted.Render(components.Truncate("d downloads leagues;", inner)),
		th.Muted.Render(components.Truncate("f removes this favourite.", inner)),
	}
}

func (m *Model) leagueLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	if c.Season.ID == "" {
		return append(lines, "", th.Empty.Render(components.Truncate("No seasons cached — press r.", inner)))
	}
	lines = append(lines, th.Muted.Render(components.Truncate(c.Competition.Country+" · "+components.SeasonName(c.Season), inner)))
	if len(c.Table) == 0 {
		lines = append(lines, "", th.Empty.Render(components.Truncate("No table cached yet.", inner)))
	}
	rows := c.Table
	// Starred clubs below the top rows replace its tail, so the card stays
	// the same height.
	if n := len(c.Starred); n > 0 {
		keep := max(tableRows-n, 1)
		if keep < len(rows) {
			rows = rows[:keep]
		}
		rows = append(append([]StandingRow{}, rows...), c.Starred...)
		if len(rows) > tableRows {
			rows = rows[:tableRows]
		}
	}
	for _, r := range rows {
		lines = append(lines, m.standingLine(r, inner))
	}
	for len(lines) < cardLines-1 {
		lines = append(lines, "")
	}
	return append(lines, m.freshness(c, inner))
}

// standingLine is "  1  Arsenal ……… 85", a starred club picked out.
func (m *Model) standingLine(r StandingRow, inner int) string {
	th := m.deps.Theme
	pos := "–"
	if r.Position != nil {
		pos = strconv.Itoa(*r.Position)
	}
	pts := strconv.Itoa(r.Points)
	name := r.Team.Name
	starred := m.deps.Favorites.Has(models.FavoriteTeam, r.Team.ID)
	if starred {
		name = "★ " + name
	}
	nameWidth := inner - 4 - len(pts) - 1
	name = components.Truncate(name, nameWidth)
	// Padded by display width, not with %-*s, which counts bytes and
	// would misalign names with accents.
	line := fmt.Sprintf("%3s ", pos) + name + strings.Repeat(" ", max(nameWidth-lipgloss.Width(name), 0)) + " " + pts
	if starred {
		return th.Accent.Bold(true).Render(line)
	}
	return th.TableCell.Render(line)
}

func (m *Model) teamLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	s := c.TeamStats
	if s == nil {
		return append(lines, "", th.Empty.Render(components.Truncate("No seasons cached.", inner)))
	}
	lines = append(lines, th.Muted.Render(components.Truncate(c.Competition.Name+" "+components.SeasonName(c.Season), inner)))

	headline := strconv.Itoa(s.Points) + " pts"
	if s.Position != nil {
		headline = ordinal(*s.Position) + " · " + headline
	}
	lines = append(lines,
		th.Accent.Bold(true).Render(components.Truncate(headline, inner)),
		th.TableCell.Render(components.Truncate(fmt.Sprintf("P%d  W%d  D%d  L%d", s.MatchesPlayed, s.Wins, s.Draws, s.Losses), inner)),
		th.TableCell.Render(components.Truncate(fmt.Sprintf("GF %d  GA %d  GD %s", s.GoalsFor, s.GoalsAgainst, components.Signed(s.GoalDiff)), inner)),
	)
	if s.XG != nil {
		diff := float64(s.GoalsFor) - *s.XG
		style := th.Positive
		if diff < 0 {
			style = th.Negative
		}
		xg := "xG " + components.Float(s.XG, 1)
		if s.XGA != nil {
			xg += "  xGA " + components.Float(s.XGA, 1)
		}
		lines = append(lines, th.TableCell.Render(xg+"  ")+style.Render("goals "+components.SignedFloat(diff, 1)))
	}
	for len(lines) < cardLines-1 {
		lines = append(lines, "")
	}
	return append(lines, m.freshness(c, inner))
}

func (m *Model) playerLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	s := c.PlayerStats
	if s == nil {
		return append(lines, "", th.Empty.Render(components.Truncate("No seasons cached.", inner)))
	}
	lines = append(lines,
		th.Muted.Render(components.Truncate(c.Team.Name+" · "+components.SeasonName(c.Season), inner)),
		th.Muted.Render(components.Truncate(c.Competition.Name, inner)),
		th.Accent.Bold(true).Render(components.Truncate(fmt.Sprintf("%d goals · %d assists", s.Goals, s.Assists), inner)),
	)
	if s.XG != nil || s.XA != nil {
		var parts []string
		if s.XG != nil {
			parts = append(parts, "xG "+components.Float(s.XG, 1))
		}
		if s.XA != nil {
			parts = append(parts, "xA "+components.Float(s.XA, 1))
		}
		lines = append(lines, th.TableCell.Render(components.Truncate(strings.Join(parts, " · "), inner)))
	}
	played := fmt.Sprintf("%d apps", s.Appearances)
	if s.Minutes > 0 {
		played += " · " + strconv.Itoa(s.Minutes) + " min"
	}
	lines = append(lines, th.TableCell.Render(components.Truncate(played, inner)))
	for len(lines) < cardLines-1 {
		lines = append(lines, "")
	}
	return append(lines, m.freshness(c, inner))
}

// freshness says how current a card's numbers are, or that they're being
// brought up to date right now.
func (m *Model) freshness(c Card, inner int) string {
	th := m.deps.Theme
	var text string
	switch {
	case c.Competition.ID != "" && c.Competition.ID == m.active:
		return th.Status.Render(components.Truncate("refreshing now…", inner))
	case m.pending[c.Competition.ID]:
		return th.Status.Render(components.Truncate("queued for refresh", inner))
	case c.LastSync == nil:
		text = "never refreshed"
	case !c.LastSync.Succeeded():
		text = "last refresh failed " + components.Freshness(c.LastSync.FinishedAt, now())
	default:
		text = "updated " + components.Freshness(c.LastSync.FinishedAt, now())
	}
	return th.Muted.Render(components.Truncate(text, inner))
}

// ordinal is 1st, 2nd, 3rd, 4th … 11th, 12th, 13th, 21st.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}
