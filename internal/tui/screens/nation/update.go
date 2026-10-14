package nation

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// Update implements core.Screen.
func (m *Model) Update(msg tea.Msg) (core.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case core.ThemeChangedMsg:
		m.deps.Theme = msg.Theme
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case core.RefreshQueueMsg:
		mine := func(r core.RefreshRequest) bool {
			return r.TeamID == m.team.ID || r.Nations == m.team.Gender
		}
		m.refreshing = msg.Active != nil && mine(*msg.Active)
		m.queued = slices.ContainsFunc(msg.Pending, mine)
		return m, nil

	case core.RefreshedMsg:
		// Any international refresh may have brought this nation matches:
		// they are shared by every nation of its gender.
		if !msg.International && !m.isOurCompetition(msg.CompetitionID) {
			return m, nil
		}
		return m, m.loadMeta()

	case metaLoadedMsg:
		wasAll := m.loaded && m.yearIdx == len(m.years)
		prev, hadYear := m.year()
		m.team = msg.team
		m.years = msg.years
		m.filter = msg.filter
		m.allComps = msg.allComps
		for _, c := range msg.allComps {
			m.compByID[c.ID] = c
		}
		// The newest year opens the screen; a reload keeps the year (or
		// "All years") the user was looking at.
		m.yearIdx = max(len(m.years)-1, 0)
		switch {
		case wasAll || len(m.years) == 0:
			m.yearIdx = len(m.years)
		case hadYear:
			if i := slices.Index(m.years, prev); i >= 0 {
				m.yearIdx = i
			}
		}
		return m, m.loadMatches()

	case matchesLoadedMsg:
		// A response for a selection the user has already moved past is
		// dropped.
		if !slices.Equal(msg.years, m.selectedYears()) || !sameFilter(msg.filter, m.filter) {
			return m, nil
		}
		selected, had := m.selected()
		m.comps = msg.comps
		for _, c := range msg.comps {
			m.compByID[c.ID] = c
		}
		m.matches = msg.matches
		m.summary = msg.summary
		m.teams = msg.teams
		m.cursor = headerRow
		for i, mt := range m.matches {
			if had && mt.ID == selected.ID {
				m.cursor = i
			}
		}
		m.filterIdx = min(m.filterIdx, len(presets)+len(m.comps)-1)
		m.loading, m.loaded, m.err = false, true, nil
		return m, nil

	case filterSavedMsg:
		if msg.err != nil {
			return m, core.Status("couldn't save the filter: " + msg.err.Error())
		}
		return m, nil

	case core.ErrMsg:
		m.err = msg.Err
		m.loading, m.loaded = false, true
		return m, nil

	case tea.KeyMsg:
		if m.focus == paneFilter {
			return m, m.filterKey(msg.String())
		}
		return m, m.matchesKey(msg.String())
	}
	return m, nil
}

func (m *Model) isOurCompetition(id string) bool {
	return slices.ContainsFunc(m.allComps, func(c models.Competition) bool { return c.ID == id })
}

func sameFilter(a, b models.MatchFilter) bool {
	return a.Preset == b.Preset && slices.Equal(a.Competitions, b.Competitions) &&
		(a.Competitions == nil) == (b.Competitions == nil)
}

func (m *Model) matchesKey(k string) tea.Cmd {
	switch k {
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-10)
	case "pgdown":
		m.move(10)
	case "home", "g":
		m.cursor = headerRow
	case "end", "G":
		m.cursor = len(m.matches) - 1
	case "left", "h":
		return m.selectYear(m.yearIdx - 1)
	case "right", "l":
		return m.selectYear(m.yearIdx + 1)
	case "tab", "shift+tab":
		m.focus = paneFilter
	case "enter":
		if mt, ok := m.selected(); ok {
			if m.expanded == mt.ID {
				m.expanded = ""
			} else {
				m.expanded = mt.ID
			}
		}
	}
	return nil
}

func (m *Model) move(delta int) {
	m.cursor = min(max(m.cursor+delta, headerRow), len(m.matches)-1)
}

// selectYear moves along the year bar, whose last stop is "All years".
func (m *Model) selectYear(idx int) tea.Cmd {
	if idx < 0 || idx > len(m.years) || idx == m.yearIdx {
		return nil
	}
	m.yearIdx = idx
	m.cursor, m.expanded = headerRow, ""
	m.loading = true
	return m.loadMatches()
}

func (m *Model) filterKey(k string) tea.Cmd {
	stops := len(presets) + len(m.comps)
	switch k {
	case "left", "h":
		m.filterIdx = max(m.filterIdx-1, 0)
	case "right", "l":
		m.filterIdx = min(m.filterIdx+1, stops-1)
	case "tab", "shift+tab":
		m.focus = paneMatches
	case " ", "x", "enter":
		if m.filterIdx < len(presets) {
			m.filter = models.MatchFilter{Preset: presets[m.filterIdx].key}
		} else if i := m.filterIdx - len(presets); i < len(m.comps) {
			m.filter = toggled(m.filter, m.comps[i], m.allComps)
		}
		m.loading = true
		return tea.Batch(m.saveFilter(), m.loadMatches())
	}
	return nil
}

// toggled flips one competition in a filter. A preset becomes the
// explicit list of the competitions it kept, minus (or plus) this one;
// and a list that comes out equal to a preset becomes that preset, so the
// filter bar shows the shorthand whenever it applies.
func toggled(f models.MatchFilter, comp models.Competition, all []models.Competition) models.MatchFilter {
	var set []string
	for _, c := range all {
		if f.Keeps(c) {
			set = append(set, c.ID)
		}
	}
	if i := slices.Index(set, comp.ID); i >= 0 {
		set = slices.Delete(set, i, i+1)
	} else {
		set = append(set, comp.ID)
	}
	for _, p := range presets {
		preset := models.MatchFilter{Preset: p.key}
		var want []string
		for _, c := range all {
			if preset.Keeps(c) {
				want = append(want, c.ID)
			}
		}
		if sameSet(set, want) {
			return preset
		}
	}
	slices.Sort(set)
	if set == nil {
		set = []string{}
	}
	return models.MatchFilter{Competitions: set}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}
