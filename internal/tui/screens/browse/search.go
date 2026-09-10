package browse

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// maxPerKind caps how many of each entity type a search returns, so one
// very common substring can't bury competitions and teams under hundreds
// of player matches.
const maxPerKind = 25

// search queries the local cache. An empty query lists the cached
// competitions, which makes the entry screen a table of contents rather
// than a blank prompt.
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
				results = append(results, Result{Kind: KindCompetition, Competition: c})
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

		// Competitions, then teams, then players: the broader the entity,
		// the more likely it is what a short query meant.
		var results []Result
		for _, c := range rank(comps, func(c models.Competition) string { return c.Name }, query) {
			results = append(results, Result{Kind: KindCompetition, Competition: c})
		}
		for _, t := range rank(teams, func(t models.Team) string { return t.Name }, query) {
			results = append(results, Result{Kind: KindTeam, Team: t})
		}
		for _, p := range rank(players, func(p models.Player) string { return p.Name }, query) {
			results = append(results, Result{Kind: KindPlayer, Player: p})
		}

		return resultsMsg{query: query, results: results}
	}
}

// rank sorts matches by how well they match and truncates to maxPerKind.
// The repository does substring matching, which alone would order
// "Manchester City" and "Manchester United" arbitrarily for the query
// "manchester united"; scoring puts the closer match first.
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

// Match quality, best first. Anything the repository returned is at least
// a substring match, so scoreSubstring is the floor.
const (
	scoreExact = iota
	scorePrefix
	scoreWordPrefix
	scoreSubstring
)

// matchScore rates how closely name matches query, lower being better.
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

// hasWordPrefix reports whether any word in name starts with query, so
// "united" ranks "Manchester United" above "Unitednordsjaelland"-style
// mid-word hits.
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
