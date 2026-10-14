// Package nation renders a national team: every match it has played in
// the years chosen, filtered down to the competitions chosen, with its
// record and averages computed from exactly those matches.
//
// That is the screen's central idea. A club's numbers come pre-summed per
// competition-season; a nation's year is a dozen matches across half a
// dozen competitions — a World Cup, its qualifiers, the Nations League,
// friendlies — and whether friendlies should count is the user's call.
// So the filter is front and centre (presets, or competitions one by one),
// is saved per nation, and every figure on screen follows it.
package nation

import (
	"slices"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// pane is which part of the screen has the keyboard.
type pane int

const (
	paneMatches pane = iota
	paneFilter
)

// headerRow is the cursor position of the nation's own header, above the
// first match. The screen opens on it, so the favourite key stars the
// nation until the user moves into the matches.
const headerRow = -1

// presets are the filter bar's first three stops, in order.
var presets = []struct {
	key, label string
}{
	{models.PresetAll, "All"},
	{models.PresetCompetitive, "Competitive"},
	{models.PresetTournaments, "Tournaments"},
}

// Model is the nation screen.
type Model struct {
	deps core.Deps
	team models.Team

	// years are the calendar years the nation has matches in, oldest
	// first; yearIdx indexes them, and len(years) is "All years".
	years   []int
	yearIdx int

	filter models.MatchFilter
	// allComps are every competition the nation has a match in, any year:
	// what an explicit filter is built from. comps are those it played in
	// the selected years: the toggles offered.
	allComps []models.Competition
	comps    []models.Competition
	compByID map[string]models.Competition

	summary models.MatchSummary
	matches []models.Match
	teams   map[string]models.Team

	cursor   int
	expanded string // the match whose box score is open
	focus    pane
	// filterIdx is the filter bar's cursor: the presets, then comps.
	filterIdx int

	// refreshing is set while this nation's matches are being fetched,
	// queued while they wait to be.
	refreshing, queued bool

	width, height int
	loading       bool
	loaded        bool
	err           error
}

// New constructs the nation screen. Loading happens in Init.
func New(deps core.Deps, team models.Team) *Model {
	return &Model{deps: deps, team: team, cursor: headerRow, loading: true, width: 100, height: 30,
		compByID: map[string]models.Competition{}, teams: map[string]models.Team{}}
}

// Title implements core.Screen.
func (m *Model) Title() string { return m.team.Name }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	if m.focus == paneFilter {
		return core.KeyMap{Short: []key.Binding{keys.FilterChoose, keys.FilterToggle, keys.FilterDone}}
	}
	return core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Year, keys.FilterBar, keys.BoxScore},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Home, keys.End},
			{keys.Year, keys.FilterBar, keys.BoxScore},
		},
	}
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd { return m.loadMeta() }

// Team is the nation on screen.
func (m *Model) Team() models.Team { return m.team }

// Filter is the competition filter in effect.
func (m *Model) Filter() models.MatchFilter { return m.filter }

// Summary is the record the filter and years give.
func (m *Model) Summary() models.MatchSummary { return m.summary }

// Matches are the matches on screen, newest first.
func (m *Model) Matches() []models.Match { return m.matches }

// FavoriteTarget implements core.Favoritable: the nation from its header
// or the filter bar, the competition of the highlighted match otherwise.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	if match, ok := m.selected(); ok && m.focus == paneMatches {
		c := m.competition(match.CompetitionID)
		return core.FavoriteTarget{Kind: models.FavoriteCompetition, ID: c.ID, Name: c.Name}, true
	}
	return core.FavoriteTarget{Kind: models.FavoriteTeam, ID: m.team.ID, Name: m.team.Label()}, true
}

// RefreshRequest implements core.RefreshRequester: the nation's matches,
// for the year on screen, or this year and last.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	req := core.RefreshRequest{TeamID: m.team.ID, Name: m.team.Label()}
	if y, ok := m.year(); ok {
		req.Seasons = []string{strconv.Itoa(y)}
	}
	return req, true
}

// Escape implements core.Escaper: esc leaves the filter bar before it
// leaves the screen.
func (m *Model) Escape() (bool, tea.Cmd) {
	if m.focus == paneFilter {
		m.focus = paneMatches
		return true, nil
	}
	return false, nil
}

// year is the selected calendar year, false for "All years".
func (m *Model) year() (int, bool) {
	if m.yearIdx < 0 || m.yearIdx >= len(m.years) {
		return 0, false
	}
	return m.years[m.yearIdx], true
}

func (m *Model) selectedYears() []int {
	if y, ok := m.year(); ok {
		return []int{y}
	}
	return nil
}

func (m *Model) selected() (models.Match, bool) {
	if m.cursor < 0 || m.cursor >= len(m.matches) {
		return models.Match{}, false
	}
	return m.matches[m.cursor], true
}

func (m *Model) competition(id string) models.Competition {
	if c, ok := m.compByID[id]; ok {
		return c
	}
	return models.Competition{ID: id, Name: id}
}

// teamName is a team's name, or its ID when it isn't cached.
func (m *Model) teamName(id string) string {
	if t, ok := m.teams[id]; ok {
		return t.Name
	}
	return id
}

// --- loading ---

type metaLoadedMsg struct {
	years    []int
	filter   models.MatchFilter
	allComps []models.Competition
	team     models.Team
}

type matchesLoadedMsg struct {
	years   []int
	filter  models.MatchFilter
	comps   []models.Competition
	matches []models.Match
	summary models.MatchSummary
	teams   map[string]models.Team
}

type filterSavedMsg struct{ err error }

// loadMeta reads what doesn't depend on the selection: the years and
// competitions the nation has matches in, and its saved filter.
func (m *Model) loadMeta() tea.Cmd {
	deps, team := m.deps, m.team
	return func() tea.Msg {
		if t, err := deps.Repos.GetTeam(deps.Ctx, team.ID); err == nil {
			team = t
		}
		years, err := deps.Repos.TeamMatchYears(deps.Ctx, team.ID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		slices.Reverse(years) // oldest first, as the bar reads
		filter, err := deps.Repos.GetTeamMatchFilter(deps.Ctx, team.ID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		all, err := deps.Repos.MatchesForTeam(deps.Ctx, team.ID, models.MatchFilter{}, nil)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		comps, err := competitionsOf(deps, all)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return metaLoadedMsg{years: years, filter: filter, allComps: comps, team: team}
	}
}

// loadMatches reads the selection: the competitions played in the chosen
// years (for the toggles), and the filtered matches and their record.
func (m *Model) loadMatches() tea.Cmd {
	deps, teamID, years, filter := m.deps, m.team.ID, m.selectedYears(), m.filter
	return func() tea.Msg {
		played, err := deps.Repos.MatchesForTeam(deps.Ctx, teamID, models.MatchFilter{}, years)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		comps, err := competitionsOf(deps, played)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		matches, err := deps.Repos.MatchesForTeam(deps.Ctx, teamID, filter, years)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		summary, err := deps.Repos.TeamMatchSummary(deps.Ctx, teamID, filter, years)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		ids := []string{}
		for _, mt := range matches {
			ids = append(ids, mt.HomeTeamID, mt.AwayTeamID)
		}
		teams, err := deps.Repos.GetTeams(deps.Ctx, ids)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return matchesLoadedMsg{years: years, filter: filter, comps: comps, matches: matches, summary: summary, teams: teams}
	}
}

// competitionsOf resolves the competitions a set of matches were played
// in, ordered tournaments, qualifiers, nations leagues, friendlies.
func competitionsOf(deps core.Deps, matches []models.Match) ([]models.Competition, error) {
	seen := map[string]bool{}
	var ids []string
	for _, mt := range matches {
		if !seen[mt.CompetitionID] {
			seen[mt.CompetitionID] = true
			ids = append(ids, mt.CompetitionID)
		}
	}
	byID, err := deps.Repos.GetCompetitions(deps.Ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]models.Competition, 0, len(ids))
	for _, id := range ids {
		c, ok := byID[id]
		if !ok {
			c = models.Competition{ID: id, Name: id}
		}
		out = append(out, c)
	}
	SortByCategory(out)
	return out, nil
}

var categoryOrder = map[string]int{
	models.CategoryTournament:    0,
	models.CategoryQualifier:     1,
	models.CategoryNationsLeague: 2,
	models.CategoryFriendly:      3,
}

// SortByCategory orders international competitions the way the filter
// reads: tournaments first, friendlies last.
func SortByCategory(comps []models.Competition) {
	slices.SortStableFunc(comps, func(a, b models.Competition) int {
		if d := categoryOrder[a.Category] - categoryOrder[b.Category]; d != 0 {
			return d
		}
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
}

// saveFilter remembers the filter for this nation. A failed save is only
// worth a status line: the filter still applies for now.
func (m *Model) saveFilter() tea.Cmd {
	deps, teamID, f := m.deps, m.team.ID, m.filter
	return func() tea.Msg {
		return filterSavedMsg{err: deps.Repos.SetTeamMatchFilter(deps.Ctx, teamID, f)}
	}
}
