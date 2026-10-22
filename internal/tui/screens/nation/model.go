// Package nation renders a national team's matches for the chosen years
// and competitions. Every figure follows the filter, which is saved per
// nation: whether friendlies count is the user's call.
package nation

import (
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/scorestrip"
)

// now is the score strip's clock; tests pin it.
var now = time.Now

type pane int

const (
	paneMatches pane = iota
	paneFilter
)

// headerRow is the nation's header, above the first match; the screen
// opens on it so favouriting stars the nation.
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

	// years is oldest first; yearIdx == len(years) is "All years".
	years   []int
	yearIdx int

	filter models.MatchFilter
	// allComps (any year) builds an explicit filter; comps (selected
	// years) are the toggles offered.
	allComps []models.Competition
	comps    []models.Competition
	compByID map[string]models.Competition

	summary models.MatchSummary
	matches []models.Match
	teams   map[string]models.Team

	cursor   int
	expanded string // the match whose box score is open
	focus    pane
	// filterIdx runs over the presets, then comps.
	filterIdx int

	refreshing, queued bool

	// strip ignores the filter.
	strip scorestrip.Strip

	width, height int
	loading       bool
	loaded        bool
	err           error
}

// New constructs the nation screen.
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
	km := core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Year, keys.FilterBar, keys.BoxScore},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown, keys.Home, keys.End},
			{keys.Year, keys.FilterBar, keys.BoxScore},
		},
	}
	if !m.strip.Empty() {
		km.Short = append(km.Short, keys.Scores)
		km.Full = append(km.Full, []key.Binding{keys.Scores})
	}
	return km
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

// FavoriteTarget implements core.Favoritable: the nation from the header
// or filter bar, else the highlighted match's competition.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	if match, ok := m.selected(); ok && m.focus == paneMatches {
		c := m.competition(match.CompetitionID)
		return core.FavoriteTarget{Kind: models.FavoriteCompetition, ID: c.ID, Name: c.Name}, true
	}
	return core.FavoriteTarget{Kind: models.FavoriteTeam, ID: m.team.ID, Name: m.team.Label()}, true
}

// RefreshRequest implements core.RefreshRequester: the year on screen, or
// this year and last.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	req := core.RefreshRequest{TeamID: m.team.ID, Name: m.team.Label()}
	if y, ok := m.year(); ok {
		req.Seasons = []string{strconv.Itoa(y)}
	}
	return req, true
}

// Escape implements core.Escaper: leave the filter bar first.
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
	strip    scorestrip.Strip
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

// loadMeta reads what doesn't depend on the selection.
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
		// The strip is a nicety: a failure leaves it empty.
		t := now()
		strip, err := scorestrip.Build(deps, scorestrip.Schedule(all, t), t)
		if err != nil {
			strip = scorestrip.Strip{}
		}
		strip.ShowComp = true
		return metaLoadedMsg{years: years, filter: filter, allComps: comps, team: team, strip: strip}
	}
}

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

// competitionsOf returns them in SortByCategory order.
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

// SortByCategory orders international competitions tournaments first,
// friendlies last.
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

// saveFilter: a failed save is only a status line; the filter still
// applies.
func (m *Model) saveFilter() tea.Cmd {
	deps, teamID, f := m.deps, m.team.ID, m.filter
	return func() tea.Msg {
		return filterSavedMsg{err: deps.Repos.SetTeamMatchFilter(deps.Ctx, teamID, f)}
	}
}
