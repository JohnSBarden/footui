package browse

import (
	"strconv"
	"strings"

	"github.com/JohnSBarden/footui/internal/tui/components"
)

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(th.Title.Render("footy"))
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
		b.WriteString(m.resultsTable())
	}

	return b.String()
}

func (m *Model) resultsTable() string {
	rows := make([][]components.Cell, 0, len(m.results))
	for _, r := range m.results {
		rows = append(rows, []components.Cell{
			components.Styled(kindLabel(r.Kind), m.deps.Theme.Muted),
			components.Text(r.Name()),
			components.Styled(m.context(r), m.deps.Theme.Muted),
		})
	}

	return components.Table(components.TableOpts{
		Columns: []components.Column{
			{Title: "Type", Width: 11},
			{Title: "Name", Width: m.nameColWidth()},
			{Title: "Details", Width: 34},
		},
		Rows:    rows,
		Cursor:  m.cursor,
		MaxRows: m.listHeight(),
		Theme:   m.deps.Theme,
	})
}

func kindLabel(k Kind) string {
	switch k {
	case KindCompetition:
		return "competition"
	case KindTeam:
		return "team"
	default:
		return "player"
	}
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

func (m *Model) nameColWidth() int {
	const fixed = 11 + 34 + 2 + 4
	w := m.width - fixed
	if w < 16 {
		return 16
	}
	if w > 34 {
		return 34
	}
	return w
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
				th.Accent.Render("footy refresh --competition "+defaultCompetition),
		}
		return strings.Join(lines, "\n")
	}
	return th.Empty.Render("No cached competition, team, or player matches that.") + "\n\n" +
		th.Muted.Render("Only downloaded competitions are searchable — press  ") +
		th.Accent.Render("ctrl+g") + th.Muted.Render("  to download more.")
}
