package dashboard

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// now is the clock freshness is measured against; tests pin it.
var now = time.Now

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder
	b.WriteString(m.header())
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
			th.Muted.Render("Press  ") + th.Accent.Render("/") + th.Muted.Render("  to search, open a league, tournament, club, nation or player,"),
			th.Muted.Render("and press  ") + th.Accent.Render("f") + th.Muted.Render("  to star it here."),
		}
		b.WriteString(strings.Join(lines, "\n"))
	default:
		b.WriteString(m.cardsView())
	}
	return b.String()
}

// header is the wordmark, what the screen is, and today's date on the
// right — the day is what "updated 3 hours ago" is measured from.
func (m *Model) header() string {
	th := m.deps.Theme
	left := components.Wordmark(th) + " " + th.Accent.Bold(true).Render("matchday") +
		th.Subtitle.Render("  your favourites")
	date := th.Muted.Render(now().Format("Mon 2 Jan"))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(date)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + date
}

// renderCard draws one card width columns wide. The selected card's
// border is drawn in the accent color and, so it still stands out without
// color, heavier.
func (m *Model) renderCard(c Card, selected bool, width int) string {
	th := m.deps.Theme
	inner := max(width-4, 8)
	var lines []string
	switch {
	case c.Missing:
		lines = m.missingLines(c, inner)
	case c.Group == GroupTournaments:
		lines = m.tournamentLines(c, inner)
	case c.Group == GroupNations:
		lines = m.nationLines(c, inner)
	case c.Kind == models.FavoriteCompetition:
		lines = m.leagueLines(c, inner)
	case c.Kind == models.FavoriteTeam:
		lines = m.teamLines(c, inner)
	default:
		lines = m.playerLines(c, inner)
	}

	style := th.Panel.Width(inner + 2)
	body := strings.Join(lines, "\n")
	if selected {
		style = style.Border(lipgloss.ThickBorder()).BorderForeground(th.Palette.Accent)
		if th.NoColor {
			style = style.UnsetBorderForeground()
		}
		return style.Render(body)
	}
	// An unselected club card wears its club's scarf along the top edge;
	// the selected one keeps the plain heavy border, so selection stays
	// the loudest thing on the screen.
	if c.Group == GroupClubs && !c.Missing {
		col := c.Colors[c.Team.ID]
		return components.ScarfTop(th, col.PrimaryHex, col.SecondaryHex, inner+4) + "\n" +
			style.BorderTop(false).Render(body)
	}
	return style.Render(body)
}

// badge says at a glance what a card is, by shape and colour: a nation's
// flag; a club's two colours as halves; ▦, a table, for a league; ◈ in
// gold for a tournament; and ◉ in the colour of a player's club.
func (m *Model) badge(c Card) string {
	th := m.deps.Theme
	switch {
	case c.Group == GroupNations || (c.Missing && strings.HasPrefix(c.ID, "nat-")):
		if f := components.Flag(c.ID); f != "" {
			return f
		}
		return th.Muted.Render("⚑")
	case c.Group == GroupLeagues:
		return th.Accent.Render("▦")
	case c.Group == GroupTournaments:
		return th.Draw.Render("◈")
	case c.Group == GroupClubs:
		return m.halves(c.Colors[c.ID])
	}
	if col, ok := styles.ClubForeground(c.Colors[c.Team.ID].PrimaryHex); ok && !th.NoColor {
		return lipgloss.NewStyle().Foreground(col).Render("◉")
	}
	return th.Accent.Render("◉")
}

// halves is a club's two colours in one cell, like a halved shirt.
func (m *Model) halves(col models.TeamColors) string {
	th := m.deps.Theme
	a, ok := styles.ClubForeground(col.PrimaryHex)
	if th.NoColor || !ok {
		return th.Faint.Render("▌")
	}
	st := lipgloss.NewStyle().Foreground(a)
	if b, ok := styles.ClubForeground(col.SecondaryHex); ok {
		st = st.Background(b)
	}
	return st.Render("▌")
}

// heading is a card's first line: its badge and name, and on the right
// how fresh its numbers are.
func (m *Model) heading(c Card, inner int) string {
	th := m.deps.Theme
	badge, age := m.badge(c), m.age(c)
	room := inner - lipgloss.Width(badge) - 1
	if age != "" {
		room -= lipgloss.Width(age) + 1
	}
	name := th.PanelTitle.Render(components.Truncate(c.Name(), room))
	if age == "" {
		return badge + " " + name
	}
	gap := max(inner-lipgloss.Width(badge)-1-lipgloss.Width(name)-lipgloss.Width(age), 1)
	return badge + " " + name + strings.Repeat(" ", gap) + age
}

func (m *Model) missingLines(c Card, inner int) []string {
	th := m.deps.Theme
	return []string{
		m.heading(c, inner),
		th.Empty.Render(components.Truncate("Not cached ("+string(c.Kind)+").", inner)),
		th.Muted.Render(components.Truncate("d downloads leagues; f un-stars it.", inner)),
	}
}

// wideTable is the inner width from which a league card's rows carry the
// whole record, not just points.
const wideTable = 56

func (m *Model) leagueLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	if c.Season.ID == "" {
		return append(lines, th.Empty.Render(components.Truncate("No seasons cached — press r.", inner)))
	}
	lines = append(lines, th.Muted.Render(components.Truncate(c.Competition.Country+" · "+components.SeasonName(c.Season), inner)))
	if len(c.Table) == 0 {
		return append(lines, th.Empty.Render(components.Truncate("No table cached yet.", inner)))
	}
	rows := c.Table
	// Starred clubs below the top rows replace its tail, so the card stays
	// the same height.
	if n := len(c.Starred); n > 0 {
		keep := max(leagueRows-n, 1)
		if keep < len(rows) {
			rows = rows[:keep]
		}
		rows = append(append([]StandingRow{}, rows...), c.Starred...)
		if len(rows) > leagueRows {
			rows = rows[:leagueRows]
		}
	}
	maxPts := 0
	for _, r := range rows {
		maxPts = max(maxPts, r.Points)
	}
	wide := inner >= wideTable
	if wide {
		lines = append(lines, m.tableHeader(inner))
	}
	for _, r := range rows {
		lines = append(lines, m.standingLine(c, r, maxPts, inner))
	}
	// A two-column card keeps its full height, so the cards flowing in
	// beneath it line up.
	for wide && len(lines) < leagueHeight-2 {
		lines = append(lines, "")
	}
	return lines
}

// pointsBarWidth is the points bar on a narrow league card's rows;
// wideBarWidth the least a wide one's gets.
const (
	pointsBarWidth = 5
	wideBarWidth   = 10
)

// recordWidth is "   P   W   D   L   GD" on a wide row: four 4-wide counts
// and a 5-wide goal difference.
const recordWidth = 4*4 + 5

// wideNameMax caps the club column on a wide row; any width past it goes
// to the points bar.
const wideNameMax = 26

// wideColumns are the club column and points bar on a wide row: what's
// left after the position and dot, the record and the points.
func wideColumns(inner int) (name, bar int) {
	name = inner - 6 - recordWidth - (2 + wideBarWidth + 1) - 3
	if name > wideNameMax {
		return wideNameMax, wideBarWidth + name - wideNameMax
	}
	return name, wideBarWidth
}

func (m *Model) tableHeader(inner int) string {
	name, bar := wideColumns(inner)
	head := "  #   " + padRight("Club", name) +
		fmt.Sprintf("%4s%4s%4s%4s%5s", "P", "W", "D", "L", "GD") +
		strings.Repeat(" ", 2+bar+1) + "Pts"
	return m.deps.Theme.Label.Render(head)
}

// standingLine is "  1 ● Arsenal ……… ███▌ 85": position, the club's dot,
// its name (a starred club picked out) and points as a bar and a number.
// A wide card adds the record between: played, won, drawn, lost and goal
// difference.
func (m *Model) standingLine(c Card, r StandingRow, maxPts, inner int) string {
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
	nameStyle := th.TableCell
	if starred {
		nameStyle = th.Accent.Bold(true)
	}
	wide := inner >= wideTable
	nameWidth, barWidth := inner-4-2-len(pts)-1-(pointsBarWidth+1), pointsBarWidth
	if wide {
		nameWidth, barWidth = wideColumns(inner)
	}
	name = components.Truncate(name, nameWidth)
	bar := th.Accent.Faint(true).Render(padBar(components.Bar(float64(r.Points), float64(maxPts), barWidth), barWidth))
	// Padded by display width, not with %-*s, which counts bytes and
	// would misalign names with accents.
	line := th.TableCell.Render(fmt.Sprintf("%3s ", pos)) +
		components.Dot(th, c.Colors[r.Team.ID].PrimaryHex) + " " +
		nameStyle.Render(name+strings.Repeat(" ", max(nameWidth-lipgloss.Width(name), 0)))
	if !wide {
		return line + " " + bar + " " + nameStyle.Render(pts)
	}
	return line +
		th.Muted.Render(fmt.Sprintf("%4d%4d%4d%4d", r.Played, r.Wins, r.Draws, r.Losses)) +
		gdStyle(th, r.GoalDiff).Render(fmt.Sprintf("%5s", components.Signed(r.GoalDiff))) +
		"  " + bar + " " + nameStyle.Render(fmt.Sprintf("%3s", pts))
}

func gdStyle(th styles.Theme, gd int) lipgloss.Style {
	switch {
	case gd > 0:
		return th.Positive
	case gd < 0:
		return th.Negative
	}
	return th.Muted
}

func (m *Model) teamLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	s := c.TeamStats
	if s == nil {
		return append(lines, th.Empty.Render(components.Truncate("No seasons cached.", inner)))
	}
	lines = append(lines, th.Muted.Render(components.Truncate(c.Competition.Name+" "+components.SeasonName(c.Season), inner)))

	headline := strconv.Itoa(s.Points) + " pts"
	headStyle := th.Accent.Bold(true)
	if s.Position != nil {
		headline = components.Ordinal(*s.Position) + " · " + headline
		// The bottom three are where a season is going wrong.
		if c.LeagueSize > 3 && *s.Position > c.LeagueSize-3 {
			headStyle = th.Negative.Bold(true)
		}
	}
	// Form sits at the headline's right when there's room for both.
	head := headStyle.Render(components.Truncate(headline, inner))
	form := components.FormChips(th, c.Summary.Form)
	if gap := inner - lipgloss.Width(head) - lipgloss.Width(form); form != "" && gap >= 2 {
		head += strings.Repeat(" ", gap) + form
		form = ""
	}
	lines = append(lines, head,
		th.TableCell.Render(components.Truncate(fmt.Sprintf("P%d  W%d  D%d  L%d  GD %s", s.MatchesPlayed, s.Wins, s.Draws, s.Losses, components.Signed(s.GoalDiff)), inner)),
	)
	if s.XG != nil {
		// Expected and actual goals on one scale, so the gap between the
		// bars is the over- or under-performance.
		scale := max(*s.XG, float64(s.GoalsFor))
		barWidth := inner - 6 - 6
		lines = append(lines,
			th.Muted.Render("xG    ")+th.Info.Render(padBar(components.Bar(*s.XG, scale, barWidth), barWidth))+" "+th.TableCell.Render(components.Float(s.XG, 1)),
			th.Muted.Render("goals ")+th.TableCell.Render(padBar(components.Bar(float64(s.GoalsFor), scale, barWidth), barWidth))+" "+th.TableCell.Render(strconv.Itoa(s.GoalsFor)),
		)
	} else {
		goals := fmt.Sprintf("GF %d · GA %d", s.GoalsFor, s.GoalsAgainst)
		lines = append(lines, th.TableCell.Render(components.Truncate(goals, inner))+
			th.Faint.Render(components.Truncate(" · no xG", max(inner-lipgloss.Width(goals), 0))))
	}
	if form != "" {
		lines = append(lines, th.Muted.Render("form ")+form)
	}
	return lines
}

func (m *Model) playerLines(c Card, inner int) []string {
	th := m.deps.Theme
	lines := []string{m.heading(c, inner)}
	s := c.PlayerStats
	if s == nil {
		return append(lines, th.Empty.Render(components.Truncate("No seasons cached.", inner)))
	}
	parts := []string{}
	if s.XG != nil {
		parts = append(parts, "xG "+components.Float(s.XG, 1))
	}
	if s.XA != nil {
		parts = append(parts, "xA "+components.Float(s.XA, 1))
	}
	parts = append(parts, fmt.Sprintf("%d apps", s.Appearances))
	// Minutes only where they fit whole.
	if mins := strconv.Itoa(s.Minutes) + " min"; s.Minutes > 0 && lipgloss.Width(strings.Join(append(parts, mins), " · ")) <= inner {
		parts = append(parts, mins)
	}
	return append(lines,
		th.Muted.Render(components.Truncate(c.Team.Name+" · "+c.Competition.Abbrev()+" "+components.SeasonName(c.Season), inner)),
		th.Accent.Bold(true).Render(components.Truncate(count(s.Goals, "goal")+" · "+count(s.Assists, "assist"), inner)),
		th.TableCell.Render(components.Truncate(strings.Join(parts, " · "), inner)),
	)
}

// age is how current a card's numbers are, short enough for its heading
// ("3h"), or that they're being brought up to date right now.
func (m *Model) age(c Card) string {
	th := m.deps.Theme
	if c.Missing {
		return ""
	}
	running, waiting := m.inFlight(c)
	switch {
	case running:
		return th.Status.Render("◐ refreshing")
	case waiting:
		return th.Faint.Render("◌ queued")
	case c.Group == GroupNations:
		if c.Updated.IsZero() {
			return ""
		}
		return th.Muted.Render(shortAge(c.Updated))
	case c.LastSync == nil:
		return th.Faint.Render("never")
	case !c.LastSync.Succeeded():
		return th.Negative.Render("⚠ " + shortAge(c.LastSync.FinishedAt))
	}
	return th.Muted.Render(shortAge(c.LastSync.FinishedAt))
}

// shortAge is how long ago something was, in one unit: "now", "5m",
// "3h", "2d", "4mo".
func shortAge(then time.Time) string {
	d := now().Sub(then)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
	return strconv.Itoa(int(d.Hours()/24/30)) + "mo"
}

// count is "1 goal", "3 goals".
func count(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// padBar pads a bar to width so the number after it lines up.
func padBar(bar string, width int) string {
	return bar + strings.Repeat(" ", max(width-lipgloss.Width(bar), 0))
}
