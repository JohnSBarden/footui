package catalog

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// sideBySideWidth is the narrowest terminal that fits both lists next to
// each other; below it the season list goes underneath.
const sideBySideWidth = 98

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(th.Title.Render(Title))
	b.WriteString(th.Subtitle.Render("  pick a league, then the seasons to fetch"))
	if n := m.downloading(); n > 0 {
		b.WriteString("  " + th.Accent.Render(fmt.Sprintf("◐ %d downloading", n)) + th.Muted.Render(" · s stops"))
	}
	b.WriteString("\n\n")

	switch {
	case m.err != nil:
		b.WriteString(th.Error.Render("Couldn't read the cache: " + m.err.Error()))
		return b.String()
	case m.loading:
		b.WriteString(th.Muted.Render("Loading…"))
		return b.String()
	case len(m.leagues) == 0:
		b.WriteString(th.Empty.Render("No competitions are available to download in this build."))
		return b.String()
	}

	left := m.leagueTable()
	right := m.seasonTable()
	if m.width >= sideBySideWidth {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right))
	} else {
		b.WriteString(left)
		b.WriteString("\n\n")
		b.WriteString(right)
	}

	b.WriteString("\n\n")
	b.WriteString(m.selectionLine())
	return b.String()
}

// downloading counts the leagues running on, or waiting in, the app's
// refresh queue.
func (m *Model) downloading() int {
	n := len(m.pending)
	if m.active != nil {
		n++
	}
	return n
}

func (m *Model) leagueTable() string {
	th := m.deps.Theme
	now := time.Now()
	rows := make([][]components.Cell, 0, len(m.leagues)+1)
	cursor := m.league
	headed := false
	for i, l := range m.leagues {
		// International entries sit under a heading of their own, below
		// the club leagues; the heading row isn't a stop for the cursor.
		if l.international() && !headed {
			headed = true
			rows = append(rows, []components.Cell{components.Styled("International", th.PanelTitle)})
			if i <= m.league {
				cursor++
			}
		}
		cached := th.Muted.Render("—")
		if n := l.cachedCount(); n > 0 {
			cached = fmt.Sprintf("%d of %d", n, len(l.Seasons))
		}
		name := l.Competition.Name
		if l.Nations == "" {
			name = m.deps.Star(models.FavoriteCompetition, l.Competition.ID, name)
		}
		country := l.Competition.Country
		if l.international() {
			country = l.Competition.Gender
		}
		rows = append(rows, []components.Cell{
			components.Text(name),
			components.Styled(country, th.Muted),
			components.Text(cached),
			components.Styled(lastRefreshed(l, now), th.Muted),
		})
	}
	return m.paneTitle("Leagues", paneLeagues) + "\n" + components.Table(components.TableOpts{
		Columns: []components.Column{
			{Title: "League", Width: 24},
			{Title: "Country", Width: 8},
			{Title: "Seasons", Width: 8},
			{Title: "Refreshed", Width: 13},
		},
		Rows:    rows,
		Cursor:  cursor,
		MaxRows: m.leagueRows(),
		Theme:   th,
		Zone:    "league",
	})
}

// leagueRows caps the league list so it fits the terminal: beside the
// season list it gets the full height, above it about half.
func (m *Model) leagueRows() int {
	h := m.height - 8
	if m.width < sideBySideWidth {
		h = (m.height - 14) / 2
	}
	return max(h, 5)
}

// lastRefreshed is the newest successful refresh across a league's seasons.
func lastRefreshed(l League, now time.Time) string {
	var newest time.Time
	for _, s := range l.Seasons {
		if s.LastRun != nil && s.LastRun.Succeeded() && s.LastRun.FinishedAt.After(newest) {
			newest = s.LastRun.FinishedAt
		}
	}
	return components.Freshness(newest, now)
}

func (m *Model) seasonTable() string {
	th := m.deps.Theme
	l, ok := m.currentLeague()
	if !ok {
		return ""
	}
	now := time.Now()
	current := ""
	if len(l.Seasons) > 0 {
		current = l.Seasons[0].Year
	}

	rows := make([][]components.Cell, 0, len(l.Seasons))
	for _, s := range l.Seasons {
		box := "[ ]"
		if m.isSelected(l.Competition.ID, s.Year) {
			box = "[x]"
		}
		rows = append(rows, []components.Cell{
			components.Styled(box, th.Accent),
			components.Text(components.SeasonLabelFor(l.Competition.CalendarYear, s.Year)),
			m.seasonStatus(l, s, current, now),
		})
	}

	cursor := -1
	if m.focus == paneSeasons {
		cursor = m.season
	}
	return m.paneTitle("Seasons — "+l.Competition.Name, paneSeasons) + "\n" + components.Table(components.TableOpts{
		Columns: []components.Column{
			{Title: "", Width: 3},
			{Title: "Season", Width: 8},
			{Title: "Status", Width: 24},
		},
		Rows:    rows,
		Cursor:  cursor,
		MaxRows: m.seasonRows(),
		Theme:   th,
		Zone:    "season",
	})
}

// seasonStatus says what's known about one season, most urgent first: in
// flight beats cached beats a past failure.
func (m *Model) seasonStatus(l League, s SeasonState, current string, now time.Time) components.Cell {
	th := m.deps.Theme
	running, waiting := m.queued(l, s.Year, current)
	switch {
	case running:
		return components.Styled("downloading…", th.Accent)
	case waiting:
		return components.Styled("queued", th.Accent)
	case s.LastRun != nil && !s.LastRun.Succeeded():
		return components.Styled("failed "+components.Freshness(s.LastRun.FinishedAt, now), th.Negative)
	case s.Cached && s.LastRun != nil:
		return components.Styled("cached "+components.Freshness(s.LastRun.FinishedAt, now), th.Positive)
	case s.Cached:
		return components.Styled("cached", th.Positive)
	}
	return components.Styled("", th.Muted)
}

func (m *Model) paneTitle(text string, p pane) string {
	if m.focus == p {
		return m.deps.Theme.PanelTitle.Render("▸ " + text)
	}
	return m.deps.Theme.Muted.Render("  " + text)
}

// seasonRows caps the season list so it fits the terminal when stacked
// under the league list.
func (m *Model) seasonRows() int {
	chrome := 8
	if m.width < sideBySideWidth {
		chrome += min(len(m.leagues)+1, m.leagueRows()) + 5
	}
	h := m.height - chrome
	if h < 4 {
		return 4
	}
	return h
}

func (m *Model) selectionLine() string {
	th := m.deps.Theme
	seasons, leagues := m.SelectedCount()
	if seasons == 0 {
		return th.Muted.Render("space selects a season · a every missing one · C this season everywhere · enter downloads")
	}
	return th.Accent.Render(fmt.Sprintf("%d season(s) selected across %d league(s)", seasons, leagues)) +
		th.Muted.Render(" — enter to download, u to clear")
}
