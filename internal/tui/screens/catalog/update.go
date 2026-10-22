package catalog

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/tui/core"
)

// Update implements core.Screen.
func (m *Model) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case core.ThemeChangedMsg:
		// Restyle content drawn for the old background.
		m.deps.Theme = msg.Theme
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case loadedMsg:
		m.leagues = msg.leagues
		m.loading = false
		m.err = nil
		m.clampCursors()
		return m, m.replayHeld()

	case core.ErrMsg:
		m.err = msg.Err
		m.loading = false
		return m, nil

	case core.RefreshQueueMsg:
		m.active, m.pending = msg.Active, msg.Pending
		return m, nil

	case core.RefreshedMsg:
		// Re-read: the refresh may have cached seasons we didn't ask for.
		return m, m.load()

	case core.ClickMsg:
		if m.loading {
			return m, nil
		}
		return m, m.click(msg)

	case tea.KeyMsg:
		if m.loading {
			m.held = append(m.held, msg)
			return m, nil
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) replayHeld() tea.Cmd {
	held := m.held
	m.held = nil
	var cmds []tea.Cmd
	for _, k := range held {
		_, cmd := m.handleKey(k)
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "tab", "shift+tab":
		m.toggleFocus()
	case "right", "l":
		m.focus = paneSeasons
	case "left", "h":
		m.focus = paneLeagues
	case " ", "x":
		if m.focus == paneLeagues {
			m.focus = paneSeasons
			return m, nil
		}
		m.toggleSeason()
	case "a":
		m.selectMissing()
	case "c":
		m.selectCurrent()
	case "C":
		m.selectCurrentEverywhere()
	case "u":
		m.selected = map[string]map[string]bool{}
	case "s":
		if m.active != nil {
			return m, core.CancelRefresh()
		}
	case "enter":
		return m, m.download()
	}
	return m, nil
}

func (m *Model) toggleFocus() {
	if m.focus == paneLeagues {
		m.focus = paneSeasons
	} else {
		m.focus = paneLeagues
	}
}

func (m *Model) move(delta int) {
	if m.focus == paneLeagues {
		m.league += delta
		m.season = 0
	} else {
		m.season += delta
	}
	m.clampCursors()
}

func (m *Model) clampCursors() {
	m.league = clamp(m.league, len(m.leagues))
	if l, ok := m.currentLeague(); ok {
		m.season = clamp(m.season, len(l.Seasons))
	} else {
		m.season = 0
	}
}

func clamp(i, n int) int {
	if i >= n {
		i = n - 1
	}
	if i < 0 {
		i = 0
	}
	return i
}

func (m *Model) isSelected(compID, year string) bool {
	return m.selected[compID][year]
}

func (m *Model) setSelected(compID, year string, on bool) {
	if on {
		if m.selected[compID] == nil {
			m.selected[compID] = map[string]bool{}
		}
		m.selected[compID][year] = true
		return
	}
	delete(m.selected[compID], year)
}

func (m *Model) toggleSeason() {
	l, ok := m.currentLeague()
	if !ok || m.season >= len(l.Seasons) {
		return
	}
	year := l.Seasons[m.season].Year
	m.setSelected(l.Competition.ID, year, !m.isSelected(l.Competition.ID, year))
	// So holding space selects a run of seasons.
	m.move(1)
}

// selectMissing toggles every uncached season: all on, or all off if they
// already were.
func (m *Model) selectMissing() {
	l, ok := m.currentLeague()
	if !ok {
		return
	}
	var missing []string
	allOn := true
	for _, s := range l.Seasons {
		if s.Cached {
			continue
		}
		missing = append(missing, s.Year)
		if !m.isSelected(l.Competition.ID, s.Year) {
			allOn = false
		}
	}
	for _, y := range missing {
		m.setSelected(l.Competition.ID, y, !allOn)
	}
}

// selectCurrent toggles the highlighted league's most recent season.
func (m *Model) selectCurrent() {
	l, ok := m.currentLeague()
	if !ok || len(l.Seasons) == 0 {
		return
	}
	y := l.Seasons[0].Year
	m.setSelected(l.Competition.ID, y, !m.isSelected(l.Competition.ID, y))
}

// selectCurrentEverywhere toggles the starter set's current seasons, like
// footui refresh --all.
func (m *Model) selectCurrentEverywhere() {
	allOn := true
	for _, l := range m.leagues {
		if l.Base && len(l.Seasons) > 0 && !m.isSelected(l.Competition.ID, l.Seasons[0].Year) {
			allOn = false
			break
		}
	}
	for _, l := range m.leagues {
		if l.Base && len(l.Seasons) > 0 {
			m.setSelected(l.Competition.ID, l.Seasons[0].Year, !allOn)
		}
	}
}

// download queues and clears the selection. With nothing selected, enter
// moves into the seasons, then selects the highlighted one.
func (m *Model) download() tea.Cmd {
	if n, _ := m.SelectedCount(); n == 0 {
		if m.focus == paneLeagues {
			m.focus = paneSeasons
			return nil
		}
		l, ok := m.currentLeague()
		if !ok || m.season >= len(l.Seasons) {
			return nil
		}
		m.setSelected(l.Competition.ID, l.Seasons[m.season].Year, true)
	}

	var reqs []core.RefreshRequest
	seasons := 0
	// Catalog order, not map order, so the queue runs in on-screen order.
	for _, l := range m.leagues {
		var years []string
		for _, s := range l.Seasons {
			if m.isSelected(l.Competition.ID, s.Year) {
				years = append(years, s.Year)
			}
		}
		if len(years) == 0 {
			continue
		}
		req := core.RefreshRequest{CompetitionID: l.Competition.ID, Seasons: years}
		if l.Nations != "" {
			req = core.RefreshRequest{Nations: l.Nations, Seasons: years, Name: l.Competition.Name}
		}
		reqs = append(reqs, req)
		seasons += len(years)
	}
	m.selected = map[string]map[string]bool{}

	return tea.Batch(
		core.RequestRefresh(reqs...),
		core.Status(fmt.Sprintf("queued %d season(s) across %d league(s) — keep browsing, progress shows below",
			seasons, len(reqs))),
	)
}

// click on the selected season ticks it, as space does.
func (m *Model) click(msg core.ClickMsg) tea.Cmd {
	if i, ok := msg.ClickedIndex("league"); ok {
		// The International heading is a row but not a league.
		if h := slices.IndexFunc(m.leagues, League.international); h >= 0 && i >= h {
			if i == h {
				return nil
			}
			i--
		}
		if i < len(m.leagues) {
			if i != m.league {
				m.season = 0
			}
			m.league, m.focus = i, paneLeagues
		}
		return nil
	}
	if i, ok := msg.ClickedIndex("season"); ok {
		l, ok := m.currentLeague()
		if !ok || i >= len(l.Seasons) {
			return nil
		}
		if m.focus == paneSeasons && i == m.season {
			year := l.Seasons[i].Year
			m.setSelected(l.Competition.ID, year, !m.isSelected(l.Competition.ID, year))
			return nil
		}
		m.season, m.focus = i, paneSeasons
	}
	return nil
}
