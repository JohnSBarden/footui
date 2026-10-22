// Package league renders a competition's standings, one cached season at a
// time.
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

// Row pairs a standings line with its team, resolved once at load time.
type Row struct {
	Stats models.TeamSeasonStats
	Team  models.Team
}

// Model is the League screen.
type Model struct {
	deps   core.Deps
	compID string
	// fromSeasonID: empty opens the most recent season.
	fromSeasonID string

	comp      models.Competition
	seasons   []models.Season
	seasonIdx int

	rows []Row
	// cursor may be headerRow.
	cursor int

	// groups is empty for a single table.
	groups   []string
	groupIdx int

	// matches are an international season's fixtures.
	matches      []models.Match
	matchTeams   map[string]models.Team
	matchesView  bool
	matchCursor  int
	matchesShown bool // the view was switched to at least once this season

	strip scorestrip.Strip

	// lastSync is nil when the season has never been refreshed.
	lastSync *models.SyncRun
	// xpts is only present where the provider has expected points.
	colors map[string]models.TeamColors
	xpts   map[string]float64

	width  int
	height int

	loading bool
	// loaded separates "still fetching" from "nothing cached".
	loaded bool
	err    error
}

// headerRow is the cursor on the league header, above the first club. The
// screen opens here so the favourite key stars the league.
const headerRow = -1

// New constructs the League screen; loading happens in Init.
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

// RefreshTarget implements core.Refreshable.
func (m *Model) RefreshTarget() (string, bool) {
	return m.compID, true
}

// --- messages ---

type competitionLoadedMsg struct {
	comp    models.Competition
	seasons []models.Season
}

type standingsLoadedMsg struct {
	seasonID   string
	rows       []Row
	matches    []models.Match
	matchTeams map[string]models.Team
	// lastSync nil (never refreshed) is a normal state.
	lastSync *models.SyncRun
	colors   map[string]models.TeamColors
	xpts     map[string]float64
	strip    scorestrip.Strip
}

// --- commands ---

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

// loadStrip reuses matches when loaded, and leaves the strip empty on
// failure rather than failing the screen.
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

// loadTable resolves team names in one batch query.
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
			// Show the ID rather than hide a row over a data defect.
			team = models.Team{ID: s.TeamID, Name: s.TeamID}
		}
		rows = append(rows, Row{Stats: s, Team: team})
	}

	// A missing sync log entry is expected, not an error.
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

func (m *Model) currentSeason() (models.Season, bool) {
	if m.seasonIdx < 0 || m.seasonIdx >= len(m.seasons) {
		return models.Season{}, false
	}
	return m.seasons[m.seasonIdx], true
}

// SelectedRow is false on the header and while fixtures are shown.
func (m *Model) SelectedRow() (Row, bool) {
	rows := m.visibleRows()
	if m.matchesView || m.cursor < 0 || m.cursor >= len(rows) {
		return Row{}, false
	}
	return rows[m.cursor], true
}
