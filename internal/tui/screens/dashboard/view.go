package dashboard

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
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
		if !m.strip.Empty() {
			b.WriteString(m.strip.View(th, m.width) + "\n\n")
		}
		if m.showLog {
			b.WriteString(m.withLog(m.cardsView(), max(m.height-headerLines-m.stripLines(), 8)))
			break
		}
		b.WriteString(m.cardsView())
	}
	return b.String()
}

func (m *Model) header() string {
	th := m.deps.Theme
	left := components.Wordmark(th) + " " + th.Accent.Bold(true).Render("matchday") +
		th.Subtitle.Render("  your favourites")
	// Short of room, the date goes before the button does.
	for _, right := range []string{m.logButton() + "  " + th.Muted.Render(now().Format("Mon 2 Jan")), m.logButton()} {
		if gap := m.width - lipgloss.Width(left) - lipgloss.Width(right); gap >= 2 {
			return left + strings.Repeat(" ", gap) + right
		}
	}
	return left
}

// renderCard gives the selected card a heavier border so it stands out
// without colour.
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
		return m.selectedCard(body, inner)
	}
	// No scarf on the selected card: selection stays the loudest thing.
	if c.Group == GroupClubs && !c.Missing {
		col := c.Colors[c.Team.ID]
		return components.ScarfTop(th, col.PrimaryHex, col.SecondaryHex, inner+4) + "\n" +
			style.BorderTop(false).Render(body)
	}
	return style.Render(body)
}

func (m *Model) badge(c Card) string {
	th := m.deps.Theme
	switch {
	case c.Group == GroupNations || (c.Missing && strings.HasPrefix(c.ID, "nat-")):
		if f := components.Flag(c.ID); f != "" {
			return f
		}
		return th.Muted.Render("⚑")
	case c.Group == GroupLeagues && components.Emoji:
		return "🥅"
	case c.Group == GroupLeagues:
		return th.Accent.Render("▦")
	case c.Group == GroupTournaments && components.Emoji:
		return "🏆"
	case c.Group == GroupTournaments:
		return th.Draw.Render("◈")
	case c.Group == GroupClubs:
		return m.inClubColour("◆", c.Colors[c.ID].PrimaryHex)
	}
	return m.inClubColour("●", c.Colors[c.Team.ID].PrimaryHex)
}

func (m *Model) inClubColour(glyph, primaryHex string) string {
	th := m.deps.Theme
	if col, ok := styles.ClubForeground(primaryHex); ok && !th.NoColor {
		return lipgloss.NewStyle().Foreground(col).Render(glyph)
	}
	return th.Accent.Render(glyph)
}

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

// wideTable is the inner width at which league rows show the full record.
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
	// Starred clubs replace the tail so the card height doesn't change.
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
	// Keep full height so cards packed beneath line up.
	for wide && len(lines) < leagueHeight-2 {
		lines = append(lines, "")
	}
	return lines
}

// Points bar widths: narrow rows, and the minimum on wide rows.
const (
	pointsBarWidth = 5
	wideBarWidth   = 10
)

// recordWidth: four 4-wide counts and a 5-wide goal difference.
const recordWidth = 4*4 + 5

// wideNameMax caps the club column; extra width goes to the points bar.
const wideNameMax = 26

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

// standingLine is "  1 ● Arsenal ……… ███▌ 85"; wide cards add the record.
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
	// Pad by display width; %-*s counts bytes and misaligns accents.
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
		if c.LeagueSize > 3 && *s.Position > c.LeagueSize-3 {
			headStyle = th.Negative.Bold(true)
		}
	}
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
		// One scale for xG and goals so the bars compare.
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
	club := c.Team.Name + " · " + c.Competition.Abbrev() + " " + components.SeasonName(c.Season)
	if c.Detail != nil {
		lines = append(lines, th.Muted.Render(components.Truncate(club, inner)))
		return append(lines, m.positionLines(c, inner)...)
	}
	parts := []string{}
	if s.XG != nil {
		parts = append(parts, "xG "+components.Float(s.XG, 1))
	}
	if s.XA != nil {
		parts = append(parts, "xA "+components.Float(s.XA, 1))
	}
	parts = append(parts, fmt.Sprintf("%d apps", s.Appearances))
	if mins := strconv.Itoa(s.Minutes) + " min"; s.Minutes > 0 && lipgloss.Width(strings.Join(append(parts, mins), " · ")) <= inner {
		parts = append(parts, mins)
	}
	return append(lines,
		th.Muted.Render(components.Truncate(club, inner)),
		th.Accent.Bold(true).Render(components.Truncate(components.Plural(s.Goals, "goal")+" · "+components.Plural(s.Assists, "assist"), inner)),
		th.TableCell.Render(components.Truncate(strings.Join(parts, " · "), inner)),
	)
}

func (m *Model) age(c Card) string {
	th := m.deps.Theme
	if c.Missing {
		return ""
	}
	if c.Kind == models.FavoritePlayer {
		// A player's season-detail refresh is keyed to them alone.
		key := core.RefreshRequest{PlayerID: c.ID}.Key()
		switch {
		case m.active[key]:
			return th.Status.Render("🔄 refreshing")
		case m.pending[key]:
			return th.Faint.Render("◌ queued")
		}
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

// shortAge uses one unit: "now", "5m", "3h", "2d", "4mo".
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

func padBar(bar string, width int) string {
	return bar + strings.Repeat(" ", max(width-lipgloss.Width(bar), 0))
}
