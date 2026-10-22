package components

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// MatchColumns are the leading columns of a team's match list; callers
// append their own last column.
func MatchColumns(opponentWidth int) []Column {
	return []Column{
		{Title: "", Width: 1},
		{Title: "Date", Width: 10}, {Title: "Comp", Width: 6}, {Title: "", Width: 3},
		{Title: "Opponent", Width: opponentWidth}, {Title: "Score", Width: 15},
	}
}

// MatchCells is one match in a team's list, from teamID's side, with last
// as the final cell. A fixture shows its kickoff instead of a score.
func MatchCells(th styles.Theme, m models.Match, teamID string, comp models.Competition, opponent, last string) []Cell {
	venue := "A"
	switch {
	case m.Neutral:
		venue = "N"
	case m.HomeTeamID == teamID:
		venue = "H"
	}
	score := ScoreFor(m, teamID)
	style := th.TableCell
	switch m.Result(teamID) {
	case "W":
		style = th.Positive
	case "L":
		style = th.Negative
	case "":
		if score == "" {
			score = Kickoff(m.Kickoff)[4:] // "14 Nov 19:45"
		}
		style = th.Muted
	}
	compStyle := th.Muted
	switch {
	case comp.Cup():
		compStyle = th.Draw
	case comp.International():
		compStyle = th.Info
	}
	glyph := " "
	if r := m.Result(teamID); r != "" {
		glyph = ResultGlyph(th, r[0])
	}
	return []Cell{
		Text(glyph),
		Text(MatchDate(m.Kickoff)),
		Styled(comp.Abbrev(), compStyle),
		Styled(venue, th.Muted),
		Text(opponent),
		Styled(score, style),
		Styled(last, th.Muted),
	}
}

// matchXGScale is the full-bar xG; few sides create more than three.
const matchXGScale = 3

// MatchXGWidth is how wide MatchXGBar's column needs to be.
const MatchXGWidth = 17

// MatchXGBar is MatchXG led by a short bar of teamID's own xG.
func MatchXGBar(th styles.Theme, m models.Match, teamID string) string {
	xg := MatchXG(m, teamID)
	if xg == "" {
		return ""
	}
	bar := ""
	if _, _, ours := m.For(teamID); ours != nil && ours.XG != nil {
		bar = Bar(*ours.XG, matchXGScale, 4)
	}
	return th.Info.Render(bar+strings.Repeat(" ", 4-lipgloss.Width(bar))) + " " + xg
}

// MatchXG is "xG 1.9–0.8" from teamID's side, or "" when the match has
// none.
func MatchXG(m models.Match, teamID string) string {
	_, _, ours := m.For(teamID)
	_, _, theirs := m.For(m.Opponent(teamID))
	if (ours == nil || ours.XG == nil) && (theirs == nil || theirs.XG == nil) {
		return ""
	}
	xg := func(s *models.MatchTeamStats) string {
		if s == nil {
			return Missing
		}
		return Float(s.XG, 1)
	}
	return "xG " + xg(ours) + "–" + xg(theirs)
}

// BoxScore is the two sides' numbers from one match, teamID's first, as
// rows of lead blank cells, a label and the pair.
func BoxScore(th styles.Theme, m models.Match, teamID string, lead int) [][]Cell {
	_, _, ours := m.For(teamID)
	_, _, theirs := m.For(m.Opponent(teamID))
	pad := func(cells ...Cell) []Cell {
		row := make([]Cell, 0, lead+len(cells))
		for range lead {
			row = append(row, Text(""))
		}
		return append(row, cells...)
	}
	if ours == nil && theirs == nil {
		return [][]Cell{pad(Styled("  no match stats", th.Muted))}
	}
	type line struct {
		label string
		f     func(*models.MatchTeamStats) string
		has   func(*models.MatchTeamStats) bool
	}
	lines := []line{
		{"Possession", func(s *models.MatchTeamStats) string {
			if s.Possession == nil {
				return Missing
			}
			return strconv.FormatFloat(*s.Possession, 'f', 0, 64) + "%"
		}, func(s *models.MatchTeamStats) bool { return s.Possession != nil }},
		{"Shots", func(s *models.MatchTeamStats) string { return Int(s.Shots) },
			func(s *models.MatchTeamStats) bool { return s.Shots != nil }},
		{"On target", func(s *models.MatchTeamStats) string { return Int(s.ShotsOnTarget) },
			func(s *models.MatchTeamStats) bool { return s.ShotsOnTarget != nil }},
		{"Corners", func(s *models.MatchTeamStats) string { return Int(s.Corners) },
			func(s *models.MatchTeamStats) bool { return s.Corners != nil }},
		{"Fouls", func(s *models.MatchTeamStats) string { return Int(s.Fouls) },
			func(s *models.MatchTeamStats) bool { return s.Fouls != nil }},
		{"xG", func(s *models.MatchTeamStats) string { return Float(s.XG, 2) },
			func(s *models.MatchTeamStats) bool { return s.XG != nil }},
	}
	get := func(s *models.MatchTeamStats, f func(*models.MatchTeamStats) string) string {
		if s == nil {
			return Missing
		}
		return f(s)
	}
	has := func(f func(*models.MatchTeamStats) bool) bool {
		return ours != nil && f(ours) || theirs != nil && f(theirs)
	}
	counts, xg := lines[:5], lines[5]
	espn := false
	for _, l := range counts {
		espn = espn || has(l.has)
	}
	// ESPN's counts show as a set when either side has any; an understat
	// match has only xG.
	var show []line
	if espn {
		show = counts
	}
	if has(xg.has) {
		show = append(show, xg)
	}
	if len(show) == 0 {
		return [][]Cell{pad(Styled("  no match stats", th.Muted))}
	}
	rows := make([][]Cell, 0, len(show))
	for _, l := range show {
		rows = append(rows, pad(
			Styled("  "+l.label, th.Muted),
			Styled(fmt.Sprintf("%s – %s", get(ours, l.f), get(theirs, l.f)), th.TableCell),
		))
	}
	return rows
}
