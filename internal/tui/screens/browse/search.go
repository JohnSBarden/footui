package browse

import (
	"errors"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/textfold"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// maxPerKind stops a common substring burying teams under players.
const maxPerKind = 25

// search queries the local cache; an empty query lists competitions.
func (m *Model) search(query string) tea.Cmd {
	deps := m.deps
	query = strings.TrimSpace(query)

	return func() tea.Msg {
		if query == "" {
			comps, err := deps.Repos.ListCompetitions(deps.Ctx)
			if err != nil {
				return core.ErrMsg{Err: err}
			}
			results := make([]Result, 0, len(comps))
			for _, c := range comps {
				// A cup has no table to open.
				if !c.Cup() {
					results = append(results, Result{Kind: KindCompetition, Competition: c})
				}
			}
			return resultsMsg{query: query, results: results}
		}

		comps, err := deps.Repos.FindCompetitionsByName(deps.Ctx, query)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		teams, err := deps.Repos.FindTeamsByName(deps.Ctx, query)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		players, err := deps.Repos.FindPlayersByName(deps.Ctx, query)
		if err != nil {
			return core.ErrMsg{Err: err}
		}

		// Broadest entity first: likelier what a short query meant.
		var results []Result
		for _, c := range rank(comps, func(c models.Competition) string { return c.Name }, query) {
			if !c.Cup() {
				results = append(results, Result{Kind: KindCompetition, Competition: c})
			}
		}
		for _, t := range rank(teams, func(t models.Team) string { return t.Name }, query) {
			results = append(results, Result{Kind: KindTeam, Team: t})
		}
		for _, p := range rank(players, func(p models.Player) string { return p.Name }, query) {
			results = append(results, Result{Kind: KindPlayer, Player: p})
		}

		// Stable, so the ranking holds within each group.
		sort.SliceStable(results, func(i, j int) bool { return groupOf(results[i]) < groupOf(results[j]) })
		var teamIDs []string
		for _, r := range results {
			if r.Kind == KindTeam {
				teamIDs = append(teamIDs, r.Team.ID)
			}
		}
		colors, err := deps.Repos.TeamColorsFor(deps.Ctx, teamIDs)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return resultsMsg{query: query, results: results, colors: colors}
	}
}

type group int

const (
	groupClubs group = iota
	groupNations
	groupPlayers
	groupLeagues
	groupFound
)

func (g group) String() string {
	return [...]string{"clubs", "nations", "players", "leagues", "not downloaded"}[g]
}

func groupOf(r Result) group {
	switch {
	case r.Kind == KindTeam && r.Team.National:
		return groupNations
	case r.Kind == KindTeam:
		return groupClubs
	case r.Kind == KindPlayer:
		return groupPlayers
	case r.Kind == KindFound:
		return groupFound
	}
	return groupLeagues
}

// searchFurtherAfter debounces source searches to one per settled query.
const searchFurtherAfter = 400 * time.Millisecond

const minFurtherQuery = 3

type searchFurtherDueMsg struct{ query string }

type foundMsg struct {
	query string
	found []Result
}

func (m *Model) scheduleFurther(query string) tea.Cmd {
	if m.deps.Players == nil || len([]rune(strings.TrimSpace(query))) < minFurtherQuery {
		return nil
	}
	return tea.Tick(searchFurtherAfter, func(time.Time) tea.Msg { return searchFurtherDueMsg{query: query} })
}

// searchFurther leaves local results alone on failure.
func (m *Model) searchFurther(query string) tea.Cmd {
	deps := m.deps
	q := strings.TrimSpace(query)
	return func() tea.Msg {
		players, err := deps.Players.SearchPlayers(deps.Ctx, q)
		if err != nil {
			return foundMsg{query: query}
		}
		var out []Result
		for _, p := range players {
			_, err := deps.Repos.ExternalRef(deps.Ctx, "player", p.Provider, p.ID)
			if err == nil {
				continue
			}
			if !errors.Is(err, storage.ErrNotFound) {
				return foundMsg{query: query}
			}
			out = append(out, Result{Kind: KindFound, Found: p})
		}
		return foundMsg{query: query, found: out}
	}
}

// merge dedupes found players by name, not ID: a cached player's source
// ID may not be linked yet.
func (m *Model) merge() {
	local := map[string]bool{}
	for _, r := range m.local {
		if r.Kind == KindPlayer {
			local[textfold.Fold(r.Player.Name)] = true
		}
	}
	m.results = append([]Result(nil), m.local...)
	for _, r := range m.found {
		if !local[textfold.Fold(r.Found.Name)] {
			m.results = append(m.results, r)
		}
	}
	m.cursor = min(m.cursor, max(len(m.results)-1, 0))
}

// rank orders substring matches by closeness and truncates to maxPerKind.
func rank[T any](items []T, name func(T) string, query string) []T {
	sorted := make([]T, len(items))
	copy(sorted, items)

	sort.SliceStable(sorted, func(i, j int) bool {
		si, sj := matchScore(name(sorted[i]), query), matchScore(name(sorted[j]), query)
		if si != sj {
			return si < sj
		}
		return name(sorted[i]) < name(sorted[j])
	})

	if len(sorted) > maxPerKind {
		sorted = sorted[:maxPerKind]
	}
	return sorted
}

// Match quality, best first; scoreSubstring is the floor.
const (
	scoreExact = iota
	scorePrefix
	scoreWordPrefix
	scoreSubstring
)

// matchScore is lower for a closer match.
func matchScore(name, query string) int {
	n, q := strings.ToLower(name), strings.ToLower(strings.TrimSpace(query))
	switch {
	case n == q:
		return scoreExact
	case strings.HasPrefix(n, q):
		return scorePrefix
	case hasWordPrefix(n, q):
		return scoreWordPrefix
	default:
		return scoreSubstring
	}
}

// hasWordPrefix reports whether any word in name starts with query.
func hasWordPrefix(name, query string) bool {
	for _, word := range strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '-' || r == '\''
	}) {
		if strings.HasPrefix(word, query) {
			return true
		}
	}
	return false
}
