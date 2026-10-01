package catalog

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/components"
)

// sideBySideWidth is the narrowest terminal that fits both lists next to
// each other; below it the season list goes underneath.
const sideBySideWidth = 96

// View implements core.Screen.
func (m *Model) View() string {
	th := m.deps.Theme
	var b strings.Builder

	b.WriteString(th.Title.Render(Title))
	b.WriteString(th.Subtitle.Render("  pick a league, then the seasons to fetch"))
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

func (m *Model) leagueTable() string {
	th := m.deps.Theme
	now := time.Now()
	rows := make([][]components.Cell, 0, len(m.leagues))
	for _, l := range m.leagues {
		cached := th.Muted.Render("—")
		if n := l.cachedCount(); n > 0 {
			cached = fmt.Sprintf("%d of %d", n, len(l.Seasons))
		}
		rows = append(rows, []components.Cell{
			components.Text(l.Competition.Name),
			components.Styled(l.Competition.Country, th.Muted),
			components.Text(cached),
			components.Styled(lastRefreshed(l, now), th.Muted),
		})
	}
	cursor := m.league
	return m.paneTitle("Leagues", paneLeagues) + "\n" + components.Table(components.TableOpts{
		Columns: []components.Column{
			{Title: "League", Width: 22},
			{Title: "Country", Width: 8},
			{Title: "Seasons", Width: 8},
			{Title: "Refreshed", Width: 13},
		},
		Rows:   rows,
		Cursor: cursor,
		Theme:  th,
	})
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
			m.seasonStatus(l.Competition.ID, s, current, now),
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
	})
}

// seasonStatus says what's known about one season, most urgent first: in
// flight beats cached beats a past failure.
func (m *Model) seasonStatus(compID string, s SeasonState, current string, now time.Time) components.Cell {
	th := m.deps.Theme
	running, waiting := m.queued(compID, s.Year, current)
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
		chrome += len(m.leagues) + 4
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
		return th.Muted.Render("space selects a season · a selects every missing one · enter downloads")
	}
	return th.Accent.Render(fmt.Sprintf("%d season(s) selected across %d league(s)", seasons, leagues)) +
		th.Muted.Render(" — enter to download")
}
