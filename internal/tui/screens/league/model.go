// Package league renders a competition's standings table, one cached
// season at a time, with every cached season selectable from a tab bar.
package league

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/scorestrip"
)

// now is the clock the scores along the top are measured against; tests
// pin it.
var now = time.Now

// Row pairs a standings stat line with the team it belongs to. Stats rows
// carry only team IDs, so the screen resolves names once at load time
// rather than per render.
type Row struct {
	Stats models.TeamSeasonStats
	Team  models.Team
}

// Model is the League screen.
type Model struct {
	deps   core.Deps
	compID string
	// fromSeasonID is the season to open on, when the user drilled in
	// from a particular one; empty opens the most recent.
	fromSeasonID string

	comp      models.Competition
	seasons   []models.Season
	seasonIdx int

	rows []Row
	// cursor is the highlighted standings row, or headerRow when the
	// league's own header is highlighted.
	cursor int

	// groups are a tournament table's groups in order, empty for one
	// table; groupIdx is the one shown.
	groups   []string
	groupIdx int

	// matches are an international season's fixtures, shown instead of
	// the table while matchesView is on.
	matches      []models.Match
	matchTeams   map[string]models.Team
	matchesView  bool
	matchCursor  int
	matchesShown bool // the view was switched to at least once this season

	// strip is the scores along the top: the season's latest round and
	// its next.
	strip scorestrip.Strip

	// lastSync is the most recent refresh attempt for the displayed
	// season, or nil when it has never been attempted.
	lastSync *models.SyncRun
	// colors are the clubs' brand colours, for their dots; xpts each
	// club's expected points, where the provider has them.
	colors map[string]models.TeamColors
	xpts   map[string]float64

	width  int
	height int

	loading bool
	// loaded distinguishes "still fetching" from "fetched, and there
	// genuinely is nothing cached" — the two need different empty states.
	loaded bool
	err    error
}

// headerRow is the cursor position of the league header, one step above
// the first club. The screen opens on it, so the favourite key stars the
// league until the user moves down into the table.
const headerRow = -1

// New constructs the League screen for a competition ID. Data loading
// happens in Init, so constructing a screen is cheap and never blocks the
// event loop.
func New(deps core.Deps, competitionID string) *Model {
	return NewAt(deps, competitionID, "")
}

// NewAt constructs the League screen opened on seasonID, falling back to
// the most recent cached season when that one isn't cached.
func NewAt(deps core.Deps, competitionID, seasonID string) *Model {
	return &Model{
		deps:         deps,
		compID:       competitionID,
		fromSeasonID: seasonID,
		cursor:       headerRow,
		loading:      true,
		width:        100,
		height:       30,
	}
}

// Title implements core.Screen.
func (m *Model) Title() string {
	if m.comp.Name != "" {
		return m.comp.Name
	}
	return m.compID
}

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	km := core.KeyMap{
		Short: []key.Binding{keys.Move, keys.Season, keys.Open},
		Full: [][]key.Binding{
			{keys.Up, keys.Down, keys.PageUp, keys.PageDown},
			{keys.Home, keys.End, keys.Season, keys.Open},
		},
	}
	if !m.strip.Empty() {
		km.Short = append(km.Short, keys.Scores)
		km.Full = append(km.Full, []key.Binding{keys.Scores})
	}
	var extra []key.Binding
	if len(m.groups) > 1 && !m.matchesView {
		extra = append(extra, keys.Group)
	}
	if !m.comp.International() {
		if len(extra) > 0 {
			km.Short = append(km.Short, extra...)
			km.Full = append(km.Full, extra)
		}
		return km
	}
	if m.matchesView {
		extra = append(extra, keys.Tables)
	} else {
		extra = append(extra, keys.Matches)
	}
	km.Short = append(km.Short, extra...)
	km.Full = append(km.Full, extra)
	return km
}

// MatchesView reports whether the screen is showing fixtures rather than
// the table.
func (m *Model) MatchesView() bool { return m.matchesView }

// Group is the tournament group shown, empty for one table.
func (m *Model) Group() string {
	if m.groupIdx < 0 || m.groupIdx >= len(m.groups) {
		return ""
	}
	return m.groups[m.groupIdx]
}

// visibleRows are the table rows on screen: one group's, or all of them.
func (m *Model) visibleRows() []Row {
	g := m.Group()
	if g == "" {
		return m.rows
	}
	var out []Row
	for _, r := range m.rows {
		if r.Stats.GroupLabel == g {
			out = append(out, r)
		}
	}
	return out
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd {
	return m.loadCompetition()
}

// FavoriteTarget implements core.Favoritable: the highlighted club, or the
// competition itself while the header is highlighted.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	if row, ok := m.SelectedRow(); ok {
		return core.FavoriteTarget{Kind: models.FavoriteTeam, ID: row.Team.ID, Name: row.Team.Name}, true
	}
	return core.FavoriteTarget{Kind: models.FavoriteCompetition, ID: m.compID, Name: m.Title()}, true
}

// HeaderSelected reports whether the cursor is on the league header
// rather than a club.
func (m *Model) HeaderSelected() bool {
	return m.cursor == headerRow
}

// RefreshTarget implements core.Refreshable: a refresh from the League
// screen fetches the competition it is showing.
func (m *Model) RefreshTarget() (string, bool) {
	return m.compID, true
}

// --- messages ---

type competitionLoadedMsg struct {
	comp    models.Competition
	seasons []models.Season
}

type standingsLoadedMsg struct {
	seasonID string
	rows     []Row
	// matches and their teams, for an international season.
	matches    []models.Match
	matchTeams map[string]models.Team
	// lastSync is nil when this competition-season has never been
	// refreshed — a normal state for data loaded before sync logging
	// existed, or seeded by hand.
	lastSync *models.SyncRun
	colors   map[string]models.TeamColors
	xpts     map[string]float64
	strip    scorestrip.Strip
}

// --- commands ---

// loadCompetition fetches the competition row and its cached seasons.
func (m *Model) loadCompetition() tea.Cmd {
	deps, compID := m.deps, m.compID
	return func() tea.Msg {
		comp, err := deps.Repos.GetCompetition(deps.Ctx, compID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		seasons, err := deps.Repos.ListSeasons(deps.Ctx, compID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		return competitionLoadedMsg{comp: comp, seasons: seasons}
	}
}

// loadStandings fetches one season's standings and resolves every team name
// in a single batch query.
func (m *Model) loadStandings(seasonID string) tea.Cmd {
	deps, compID, international := m.deps, m.compID, m.comp.International()
	return func() tea.Msg {
		msg, err := loadTable(deps, compID, seasonID)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		if international {
			if msg.matches, err = deps.Repos.MatchesForCompetitionSeason(deps.Ctx, compID, seasonID); err != nil {
				return core.ErrMsg{Err: err}
			}
			ids := make([]string, 0, 2*len(msg.matches))
			for _, mt := range msg.matches {
				ids = append(ids, mt.HomeTeamID, mt.AwayTeamID)
			}
			if msg.matchTeams, err = deps.Repos.GetTeams(deps.Ctx, ids); err != nil {
				return core.ErrMsg{Err: err}
			}
		}
		msg.strip = loadStrip(deps, compID, seasonID, msg.matches)
		return msg
	}
}

// loadStrip is the season's latest round and its next, from matches when
// they're already loaded. The strip is a nicety, so a failure leaves it
// empty rather than failing the screen.
func loadStrip(deps core.Deps, compID, seasonID string, matches []models.Match) scorestrip.Strip {
	t := now()
	var round []models.Match
	if matches != nil {
		round = scorestrip.Rounds(matches, t)
	} else {
		var err error
		if round, err = scorestrip.ForSeason(deps, compID, seasonID, t); err != nil {
			return scorestrip.Strip{}
		}
	}
	strip, err := scorestrip.Build(deps, round, t)
	if err != nil {
		return scorestrip.Strip{}
	}
	if deps.Favorites != nil {
		strip.Highlight = func(teamID string) bool { return deps.Favorites.Has(models.FavoriteTeam, teamID) }
	}
	return strip
}

// loadTable is one season's standings with every team name resolved in
// a single batch query.
func loadTable(deps core.Deps, compID, seasonID string) (standingsLoadedMsg, error) {
	stats, err := deps.Repos.Standings(deps.Ctx, compID, seasonID)
	if err != nil {
		return standingsLoadedMsg{}, err
	}

	ids := make([]string, 0, len(stats))
	for _, s := range stats {
		ids = append(ids, s.TeamID)
	}
	teams, err := deps.Repos.GetTeams(deps.Ctx, ids)
	if err != nil {
		return standingsLoadedMsg{}, err
	}

	rows := make([]Row, 0, len(stats))
	for _, s := range stats {
		team, ok := teams[s.TeamID]
		if !ok {
			// Render the ID rather than dropping the row: a missing
			// team row is a data defect worth seeing, not worth
			// hiding a league position over.
			team = models.Team{ID: s.TeamID, Name: s.TeamID}
		}
		rows = append(rows, Row{Stats: s, Team: team})
	}

	// A missing sync log entry is expected, not an error, so it never
	// fails the load.
	var lastSync *models.SyncRun
	if run, err := deps.Repos.LatestSyncRun(deps.Ctx, compID, seasonID); err == nil {
		lastSync = &run
	}
	colors, err := deps.Repos.TeamColorsFor(deps.Ctx, ids)
	if err != nil {
		return standingsLoadedMsg{}, err
	}
	xpts, err := deps.Repos.SeasonStatExtra(deps.Ctx, compID, seasonID, models.StatExpectedPoints)
	if err != nil {
		return standingsLoadedMsg{}, err
	}

	return standingsLoadedMsg{seasonID: seasonID, rows: rows, lastSync: lastSync, colors: colors, xpts: xpts}, nil
}

// currentSeason returns the season currently being displayed, if any.
func (m *Model) currentSeason() (models.Season, bool) {
	if m.seasonIdx < 0 || m.seasonIdx >= len(m.seasons) {
		return models.Season{}, false
	}
	return m.seasons[m.seasonIdx], true
}

// SelectedRow returns the row under the cursor, for drill-down. It is
// false while the header is highlighted, and while fixtures are shown.
func (m *Model) SelectedRow() (Row, bool) {
	rows := m.visibleRows()
	if m.matchesView || m.cursor < 0 || m.cursor >= len(rows) {
		return Row{}, false
	}
	return rows[m.cursor], true
}
