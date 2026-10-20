package league

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// A tournament season is shown either as its group tables or as every
// fixture by round. Fixtures are grouped into stages — the group stage,
// then each knockout round — in the order they were played; within the
// group stage they run group by group, which is how a reader follows one.

// stageOf is the section a match is listed under.
func stageOf(m models.Match) string { return components.Stage(m) }

// displayOrder sorts a season's matches into the order the fixtures view
// lists them: stage by stage, each stage where its first match falls.
func displayOrder(matches []models.Match) []models.Match {
	first := map[string]int{}
	for i, m := range matches { // matches arrive in kickoff order
		if _, ok := first[stageOf(m)]; !ok {
			first[stageOf(m)] = i
		}
	}
	out := slices.Clone(matches)
	slices.SortStableFunc(out, func(a, b models.Match) int {
		if d := first[stageOf(a)] - first[stageOf(b)]; d != 0 {
			return d
		}
		if stageOf(a) == components.GroupStage && a.Round != b.Round {
			return strings.Compare(a.Round, b.Round)
		}
		return a.Kickoff.Compare(b.Kickoff)
	})
	return out
}

// fixturesView lists every fixture of the season under its stage, scrolled
// so the cursor's match is on screen.
func (m *Model) fixturesView() string {
	th := m.deps.Theme
	if len(m.matches) == 0 {
		season, _ := m.currentSeason()
		return m.emptyState("No fixtures cached for " + components.SeasonName(season) + ".")
	}
	name := func(id string) string {
		if t, ok := m.matchTeams[id]; ok {
			return m.deps.Star(models.FavoriteTeam, id, t.Name)
		}
		return id
	}
	const sideWidth = 22
	type line struct {
		text  string
		match int // -1 for a heading
	}
	var lines []line
	stage := ""
	for i, mt := range m.matches {
		if s := stageOf(mt); s != stage {
			if stage != "" {
				lines = append(lines, line{"", -1})
			}
			stage = s
			lines = append(lines, line{components.Label(th, s), -1})
		}
		score := components.Score(mt)
		if score == "" {
			score = components.Kickoff(mt.Kickoff)
		}
		round := ""
		if stage == components.GroupStage {
			round = strings.TrimPrefix(mt.Round, "Group ")
		}
		text := "  " + padTo(components.MatchDate(mt.Kickoff), 10) + "  " + padTo(round, 3) + " " +
			padLeft(name(mt.HomeTeamID), sideWidth) + "  " + center(score, 16) + "  " + padTo(name(mt.AwayTeamID), sideWidth)
		lines = append(lines, line{text, i})
	}

	// Keep the cursor's line in view, with its stage heading where room
	// allows.
	at := 0
	for i, l := range lines {
		if l.match == m.matchCursor {
			at = i
		}
	}
	// A line is kept for "fixture n of m" under them.
	height := max(m.tableHeight()-1, 3)
	start := min(max(at-height/2, 0), max(len(lines)-height, 0))
	end := min(start+height, len(lines))
	var out []string
	for _, l := range lines[start:end] {
		switch {
		case l.match == m.matchCursor:
			out = append(out, th.TableSelected.Render(l.text))
		case l.match >= 0:
			out = append(out, th.TableCell.Render(l.text))
		default:
			out = append(out, l.text)
		}
	}
	if start > 0 || end < len(lines) {
		out = append(out, th.Muted.Render("  "+components.Truncate(
			"fixture "+strconv.Itoa(m.matchCursor+1)+" of "+strconv.Itoa(len(m.matches)), m.width)))
	}
	return strings.Join(out, "\n")
}

// groupBar is "Groups  A  [B]  C …", the short form of each group's name.
func (m *Model) groupBar() string {
	if len(m.groups) < 2 {
		return ""
	}
	th := m.deps.Theme
	parts := []string{th.Muted.Render("Groups ")}
	for i, g := range m.groups {
		label := strings.TrimPrefix(g, "Group ")
		if i == m.groupIdx {
			parts = append(parts, th.Accent.Bold(true).Render("["+label+"]"))
		} else {
			parts = append(parts, th.Muted.Render(" "+label+" "))
		}
	}
	return components.Truncate(strings.Join(parts, " "), max(m.width-2, 10))
}

func padTo(s string, w int) string {
	s = components.Truncate(s, w)
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}

func padLeft(s string, w int) string {
	s = components.Truncate(s, w)
	return strings.Repeat(" ", max(w-lipgloss.Width(s), 0)) + s
}

func center(s string, w int) string {
	s = components.Truncate(s, w)
	gap := max(w-lipgloss.Width(s), 0)
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}
