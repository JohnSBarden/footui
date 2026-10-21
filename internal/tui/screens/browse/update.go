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
		// This screen draws its own content, so it adopts the new theme
		// rather than leaving text styled for the old background.
		m.deps.Theme = msg.Theme
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case core.RefreshedMsg:
		// Whatever the query, the index it searches just changed.
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
		// Once the downloads have started and the queue drains, a player
		// still not found won't be: the source's squads for those seasons
		// didn't have them.
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

	// Everything else is text entry. The query is re-run only when the
	// text actually changed, so arrow keys inside the input don't trigger
	// redundant queries.
	before := m.input.Value()
	updated, inputCmd := m.input.Update(msg)
	m.input = updated

	if after := m.input.Value(); after != before {
		m.newQuery(after)
		return m, tea.Batch(inputCmd, m.search(after))
	}
	return m, inputCmd
}

// handleListKey handles keys while the results list has focus. The search
// box isn't listening, so vim-style movement is safe here.
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

// open drills into whatever the selected result points at.
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
		// Browse has no competition/season context to pass on; the Team
		// screen defaults to the most recent season of the club's own
		// league.
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

// newQuery starts a search for query, forgetting the last one's results
// from the data source.
func (m *Model) newQuery(query string) {
	m.query = query
	m.loading = true
	m.searched = false
	m.found = nil
	m.searchingFurther = false
}

// maxCareerSeasons caps how much of a career one enter downloads: the
// most recent seasons are what a search for a player is usually after,
// and each season is a whole league's download.
const maxCareerSeasons = 12

// careerMsg is a found player's career, looked up to download it.
type careerMsg struct {
	player  models.FoundPlayer
	seasons []models.CareerSeason
	err     error
}

// downloadCareer queues the seasons of a found player's career that
// footui covers and has player stats for — club leagues and Europe, not
// cups or internationals — newest first, skipping the cached ones.
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

// cached reports whether a competition-season is already in the cache.
func (m *Model) cached(s models.CareerSeason) bool {
	seasons, err := m.deps.Repos.ListSeasons(m.deps.Ctx, s.CompetitionID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(seasons, func(c models.Season) bool { return c.Label == s.Year })
}

// locatedMsg is the awaited player, now cached.
type locatedMsg struct{ player models.Player }

// notLocatedMsg is the awaited player still missing after every download.
type notLocatedMsg struct{ player models.FoundPlayer }

// locateAwaited looks for the player whose career is downloading: by the
// source's ID once a sync has linked it, otherwise by their exact name.
// final is the last look, once the downloads are done, which reports a
// miss rather than staying quiet.
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
