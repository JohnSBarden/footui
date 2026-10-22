package browse

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/textfold"
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

	case core.RefreshedMsg:
		m.loading = true
		return m, tea.Batch(m.search(m.query), m.locateAwaited(false))

	case resultsMsg:
		// Drop results for a query the user has already typed past.
		if msg.query != m.query {
			return m, nil
		}
		fresh := !m.searched
		m.searched = true
		m.local = msg.results
		m.colors = msg.colors
		if fresh {
			m.cursor = 0
		}
		m.merge()
		m.loading = false
		if fresh {
			m.searchingFurther = m.deps.Players != nil && len([]rune(strings.TrimSpace(m.query))) >= minFurtherQuery
			return m, m.scheduleFurther(m.query)
		}
		return m, nil

	case searchFurtherDueMsg:
		if msg.query != m.query {
			return m, nil
		}
		return m, m.searchFurther(msg.query)

	case foundMsg:
		if msg.query != m.query {
			return m, nil
		}
		m.found = msg.found
		m.searchingFurther = false
		m.merge()
		return m, nil

	case careerMsg:
		return m, m.downloadCareer(msg)

	case core.RefreshQueueMsg:
		// Queue drained after downloads started: a miss is final.
		busy := msg.Active != nil || len(msg.Pending) > 0
		if m.awaiting != nil && busy {
			m.awaitStarted = true
		}
		if m.awaiting != nil && m.awaitStarted && !busy {
			return m, m.locateAwaited(true)
		}
		return m, nil

	case notLocatedMsg:
		if m.awaiting == nil || m.awaiting.ID != msg.player.ID {
			return m, nil
		}
		m.awaiting = nil
		return m, core.Status(msg.player.Name + " wasn't in the squads of the seasons downloaded")

	case locatedMsg:
		m.awaiting = nil
		for i, r := range m.results {
			if r.Kind == KindPlayer && r.Player.ID == msg.player.ID {
				m.cursor = i
			}
		}
		return m, core.Status(msg.player.Name + " is ready — enter opens their page")

	case core.ErrMsg:
		m.err = msg.Err
		m.loading = false
		return m, nil

	case core.ClickMsg:
		// A click selects a result; clicking the selected one opens it.
		i, ok := msg.ClickedIndex("result")
		if !ok || i >= len(m.results) {
			return m, nil
		}
		if i == m.cursor {
			return m, m.open()
		}
		m.cursor = i
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	if !m.input.Focused() {
		return m.handleListKey(msg)
	}
	switch msg.String() {
	case "tab":
		if len(m.results) > 0 {
			m.input.Blur()
		}
		return m, nil
	case "up", "ctrl+p":
		m.moveCursor(-1)
		return m, nil
	case "down", "ctrl+n":
		m.moveCursor(1)
		return m, nil
	case "pgup":
		m.moveCursor(-10)
		return m, nil
	case "pgdown":
		m.moveCursor(10)
		return m, nil
	case "enter":
		return m, m.open()
	}

	// Re-run only when the text changed, not on cursor movement.
	before := m.input.Value()
	updated, inputCmd := m.input.Update(msg)
	m.input = updated

	if after := m.input.Value(); after != before {
		m.newQuery(after)
		return m, tea.Batch(inputCmd, m.search(after))
	}
	return m, inputCmd
}

// handleListKey: the box isn't listening, so vim-style keys are safe.
func (m *Model) handleListKey(msg tea.KeyMsg) (core.Screen, tea.Cmd) {
	switch msg.String() {
	case "up", "k", "ctrl+p":
		m.moveCursor(-1)
	case "down", "j", "ctrl+n":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-10)
	case "pgdown":
		m.moveCursor(10)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.moveCursor(len(m.results))
	case "tab", "/":
		return m, m.input.Focus()
	case "enter":
		return m, m.open()
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	if len(m.results) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.results)-1)
}

func (m *Model) open() tea.Cmd {
	result, ok := m.SelectedResult()
	if !ok {
		return nil
	}

	switch result.Kind {
	case KindCompetition:
		if m.deps.Nav.League == nil {
			return nil
		}
		return core.Push(m.deps.Nav.League(m.deps, result.Competition.ID, ""))

	case KindTeam:
		if m.deps.Nav.Team == nil {
			return nil
		}
		// No context: Team defaults to its league's latest season.
		return core.Push(m.deps.Nav.Team(m.deps, result.Team, "", ""))

	case KindFound:
		if m.deps.Players == nil {
			return nil
		}
		deps, found := m.deps, result.Found
		return tea.Batch(
			core.Status("looking up "+found.Name+"'s career…"),
			func() tea.Msg {
				seasons, err := deps.Players.PlayerCareer(deps.Ctx, found.ID)
				return careerMsg{player: found, seasons: seasons, err: err}
			})

	default:
		if m.deps.Nav.Player == nil {
			return nil
		}
		return core.Push(m.deps.Nav.Player(m.deps, result.Player, ""))
	}
}

func (m *Model) newQuery(query string) {
	m.query = query
	m.loading = true
	m.searched = false
	m.found = nil
	m.searchingFurther = false
}

// maxCareerSeasons caps one enter's download; each season is a whole
// league.
const maxCareerSeasons = 12

type careerMsg struct {
	player  models.FoundPlayer
	seasons []models.CareerSeason
	err     error
}

// downloadCareer queues uncached club-league and European seasons, newest
// first; cups and internationals have no player stats.
func (m *Model) downloadCareer(msg careerMsg) tea.Cmd {
	if msg.err != nil {
		return core.Status("couldn't look up " + msg.player.Name + ": " + msg.err.Error())
	}
	byComp := map[string]core.CatalogEntry{}
	for _, e := range m.deps.Catalog {
		if e.Nations == "" && !e.ByYear && !e.Competition.Cup() && !e.Competition.International() {
			byComp[e.Competition.ID] = e
		}
	}
	var reqs []core.RefreshRequest
	at := map[string]int{}
	n := 0
	for _, s := range msg.seasons {
		e, ok := byComp[s.CompetitionID]
		if !ok || !slices.Contains(e.Seasons, s.Year) || m.cached(s) {
			continue
		}
		if n == maxCareerSeasons {
			break
		}
		n++
		i, ok := at[s.CompetitionID]
		if !ok {
			i = len(reqs)
			at[s.CompetitionID] = i
			reqs = append(reqs, core.RefreshRequest{CompetitionID: s.CompetitionID, Name: e.Competition.Name})
		}
		reqs[i].Seasons = append(reqs[i].Seasons, s.Year)
	}
	if n == 0 {
		return core.Status("none of " + msg.player.Name + "'s seasons are in leagues footui can download")
	}
	found := msg.player
	m.awaiting, m.awaitStarted = &found, false
	return tea.Batch(
		core.RequestRefresh(reqs...),
		core.Status(fmt.Sprintf("downloading %d season(s) of %s's career — they'll show here when the first lands", n, found.Name)),
	)
}

func (m *Model) cached(s models.CareerSeason) bool {
	seasons, err := m.deps.Repos.ListSeasons(m.deps.Ctx, s.CompetitionID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(seasons, func(c models.Season) bool { return c.Label == s.Year })
}

type locatedMsg struct{ player models.Player }

type notLocatedMsg struct{ player models.FoundPlayer }

// locateAwaited matches by source ID once linked, else exact name. final
// reports a miss rather than staying quiet.
func (m *Model) locateAwaited(final bool) tea.Cmd {
	if m.awaiting == nil {
		return nil
	}
	deps, want := m.deps, *m.awaiting
	return func() tea.Msg {
		var miss tea.Msg
		if final {
			miss = notLocatedMsg{player: want}
		}
		if id, err := deps.Repos.ExternalRef(deps.Ctx, "player", want.Provider, want.ID); err == nil {
			if p, err := deps.Repos.GetPlayer(deps.Ctx, id); err == nil {
				return locatedMsg{player: p}
			}
		}
		players, err := deps.Repos.FindPlayersByName(deps.Ctx, want.Name)
		if err != nil {
			return miss
		}
		var exact []models.Player
		for _, p := range players {
			if textfold.Fold(p.Name) == textfold.Fold(want.Name) {
				exact = append(exact, p)
			}
		}
		if len(exact) != 1 {
			return miss
		}
		return locatedMsg{player: exact[0]}
	}
}
