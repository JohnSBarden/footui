package browse

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
)

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(components.Wordmark(th))
	b.WriteString(th.Subtitle.Render("  search what you've cached locally"))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(th.Error.Render("Search failed: " + m.err.Error()))
	case m.loading:
		b.WriteString(th.Muted.Render("Searching…"))
	case len(m.results) == 0:
		b.WriteString(m.emptyState())
	default:
		b.WriteString(m.resultsList())
	}

	return b.String()
}

// line is one row of the results list: a group heading (result -1) or a
// result.
type line struct {
	text   string
	result int
}

// resultsList draws the results under their group headings, each with its
// club's dot and its context on the right, scrolled so the cursor's row
// is on screen.
func (m *Model) resultsList() string {
	th := m.deps.Theme
	var lines []line
	counts := map[group]int{}
	for _, r := range m.results {
		counts[groupOf(r)]++
	}
	for i, r := range m.results {
		if g := groupOf(r); i == 0 || g != groupOf(m.results[i-1]) {
			if i > 0 {
				lines = append(lines, line{result: -1})
			}
			lines = append(lines, line{text: components.Label(th, g.String()) + " " + th.Faint.Render(strconv.Itoa(counts[g])), result: -1})
		}
		lines = append(lines, line{text: m.resultLine(r, i == m.cursor), result: i})
	}

	height := m.listHeight()
	cursorLine := 0
	for i, l := range lines {
		if l.result == m.cursor {
			cursorLine = i
		}
	}
	start := max(0, cursorLine-height+1)
	end := min(len(lines), start+height)
	out := make([]string, 0, end-start+1)
	for _, l := range lines[start:end] {
		out = append(out, l.text)
	}
	// A cut list says so, so a scrolled view isn't mistaken for every match.
	if start > 0 || end < len(lines) {
		out = append(out, th.Muted.Render("  showing "+strconv.Itoa(end-start)+" of "+strconv.Itoa(len(lines))+" lines"))
	}
	return strings.Join(out, "\n")
}

// resultLine is "  ● Manchester United ★ ←        England · men": the
// club's dot, the name, the selection marker and the context.
func (m *Model) resultLine(r Result, selected bool) string {
	th := m.deps.Theme
	kind, id := r.favoriteKey()
	dot := ""
	if r.Kind == KindTeam {
		dot = m.colors[r.Team.ID].PrimaryHex
	}
	meta := m.context(r)
	name := m.deps.Star(kind, id, r.Name())
	marker := ""
	if selected {
		marker = " ←"
	}
	// Names keep their room; the context gives way first.
	room := m.width - 6 - lipgloss.Width(marker)
	if lipgloss.Width(name)+2+lipgloss.Width(meta) > room {
		meta = components.Truncate(meta, max(room-lipgloss.Width(name)-2, 0))
	}
	name = components.Truncate(name, room)
	nameStyle := th.TableCell
	if selected {
		nameStyle = th.TableSelected
	}
	left := "  " + components.Dot(th, dot) + " " + nameStyle.Render(name) + th.Accent.Bold(true).Render(marker)
	gap := max(m.width-2-lipgloss.Width(left)-lipgloss.Width(meta), 1)
	if meta == "" {
		return left
	}
	return left + strings.Repeat(" ", gap) + th.Muted.Render(meta)
}

// context is the trailing detail that disambiguates same-named results —
// which country a club is in, which position a player plays.
func (m *Model) context(r Result) string {
	switch r.Kind {
	case KindCompetition:
		parts := []string{r.Competition.Country}
		if r.Competition.Tier > 0 {
			parts = append(parts, "tier "+strconv.Itoa(r.Competition.Tier))
		}
		if r.Competition.International() {
			parts = append(parts, r.Competition.Category)
		}
		if r.Competition.Gender != "" {
			parts = append(parts, r.Competition.Gender)
		}
		return strings.Join(parts, " · ")
	case KindTeam:
		parts := []string{}
		if r.Team.Country != "" {
			parts = append(parts, r.Team.Country)
		}
		if r.Team.Gender != "" {
			parts = append(parts, r.Team.Gender)
		}
		return strings.Join(parts, " · ")
	default:
		return r.Player.Position
	}
}

func (m *Model) listHeight() int {
	const chrome = 10
	h := m.height - chrome
	if h < 5 {
		return 5
	}
	return h
}

func (m *Model) emptyState() string {
	th := m.deps.Theme
	if strings.TrimSpace(m.input.Value()) == "" {
		// Nothing cached at all — this is a first run.
		//
		// Each line is styled separately and joined afterwards: a newline
		// inside a single Render call makes Lip Gloss pad every line to
		// the width of the longest, which leaves the shorter one visibly
		// indented.
		lines := []string{
			th.Empty.Render("Nothing cached yet."),
			"",
			th.Muted.Render("Press  ") + th.Accent.Render("ctrl+g") +
				th.Muted.Render("  to choose leagues and seasons to download,"),
			th.Muted.Render("or  ") + th.Accent.Render("ctrl+r") +
				th.Muted.Render("  to fetch this season of the Premier League now."),
			th.Muted.Render("From a shell:  ") +
				th.Accent.Render("footui refresh --competition "+defaultCompetition),
		}
		return strings.Join(lines, "\n")
	}
	return th.Empty.Render("No cached competition, team, or player matches that.") + "\n\n" +
		th.Muted.Render("Only downloaded competitions are searchable — press  ") +
		th.Accent.Render("ctrl+g") + th.Muted.Render("  to download more.")
}
