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
				// A cup is stored for its matches, which show on a
				// club's screen; it has no table to open.
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

		// Competitions, then teams, then players: the broader the entity,
		// the more likely it is what a short query meant.
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

		// Shown grouped — clubs, nations, players, leagues — with the
		// ranking above kept within each group.
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

// group is a heading the results are shown under.
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

// groupOf is the heading a result is shown under.
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

// searchFurtherAfter is how long typing must pause before the data
// source is searched too: one request per settled query, not per key.
const searchFurtherAfter = 400 * time.Millisecond

// minFurtherQuery is the shortest query worth sending to the source.
const minFurtherQuery = 3

// searchFurtherDueMsg is the pause after typing running out.
type searchFurtherDueMsg struct{ query string }

// foundMsg is what the data source's search turned up for query.
type foundMsg struct {
	query string
	found []Result
}

// scheduleFurther asks for the data source to be searched once typing
// pauses, if there is one and the query is long enough to mean someone.
func (m *Model) scheduleFurther(query string) tea.Cmd {
	if m.deps.Players == nil || len([]rune(strings.TrimSpace(query))) < minFurtherQuery {
		return nil
	}
	return tea.Tick(searchFurtherAfter, func(time.Time) tea.Msg { return searchFurtherDueMsg{query: query} })
}

// searchFurther searches the data source for players, keeping those not
// already cached. A failure leaves the local results as they are: the
// source being unreachable is no reason to hide what is cached.
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

// merge rebuilds the list from the local results and the found players
// that aren't cached under the same name: a player cached from a source
// that hasn't linked its ID yet is still the same player.
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
