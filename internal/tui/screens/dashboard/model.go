// Package dashboard is the landing page once anything is favourited: a
// card per favourite league, club and player, each showing where it
// stands this season, and each drilling into the screen Browse would
// open.
//
// Cards are grouped in a fixed order — leagues, then clubs, then players —
// and within each group by when they were starred. Search is one key away
// (/), so the dashboard replaces the bare search box without hiding it.
package dashboard

import (
	"errors"
	"sort"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
)

// Title is the dashboard's breadcrumb title.
const Title = "Dashboard"

// tableRows is how many standings rows a league card shows.
const tableRows = 5

// StandingRow is one line of a league card's table.
type StandingRow struct {
	Position *int
	Team     models.Team
	Points   int
	Played   int
}

// Card is one favourite's card. Kind selects which of the fields below
// mean anything.
type Card struct {
	Kind models.FavoriteKind
	ID   string
	// Missing is set when the favourite's entity isn't cached (it was
	// starred, then its data deleted with the database file, say). The
	// card stays, so it can still be un-starred.
	Missing bool

	// Competition and Season are where the card's numbers come from: the
	// league itself, or the competition the club or player was last seen
	// in. Season is zero when nothing is cached for it.
	Competition models.Competition
	Season      models.Season
	// LastSync is the latest refresh of that competition-season, nil if
	// it was never refreshed.
	LastSync *models.SyncRun

	// League cards.
	Table []StandingRow
	// Teams in this league the user has starred that fell outside the top
	// rows, shown beneath them so a favourite club is always visible.
	Starred []StandingRow

	// Club cards.
	Team      models.Team
	TeamStats *models.TeamSeasonStats

	// Player cards. Team is the club the line is for.
	Player      models.Player
	PlayerStats *models.PlayerSeasonStats
}

// Name is the card's heading.
func (c Card) Name() string {
	switch {
	case c.Missing:
		return c.ID
	case c.Kind == models.FavoriteCompetition:
		return c.Competition.Name
	case c.Kind == models.FavoriteTeam:
		return c.Team.Name
	default:
		return c.Player.Name
	}
}

// Model is the dashboard screen.
type Model struct {
	deps core.Deps

	cards  []Card
	cursor int

	// active and pending are the competitions being refreshed and waiting
	// to be, so a card can say its numbers are on their way.
	active  string
	pending map[string]bool

	width  int
	height int

	loading bool
	err     error
}

// New constructs the dashboard. Cards load in Init.
func New(deps core.Deps) *Model {
	return &Model{deps: deps, loading: true, width: 100, height: 30, pending: map[string]bool{}}
}

// Title implements core.Screen.
func (m *Model) Title() string { return Title }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	return core.KeyMap{
		Short: []key.Binding{keys.MoveCard, keys.Open},
	}
}

// Init implements core.Screen.
func (m *Model) Init() tea.Cmd { return m.load() }

// Cards returns the cards in display order.
func (m *Model) Cards() []Card { return m.cards }

// Selected returns the card under the cursor.
func (m *Model) Selected() (Card, bool) {
	if m.cursor < 0 || m.cursor >= len(m.cards) {
		return Card{}, false
	}
	return m.cards[m.cursor], true
}

// FavoriteTarget implements core.Favoritable: the selected card, so a
// favourite can be removed without leaving the dashboard.
func (m *Model) FavoriteTarget() (core.FavoriteTarget, bool) {
	c, ok := m.Selected()
	if !ok {
		return core.FavoriteTarget{}, false
	}
	return core.FavoriteTarget{Kind: c.Kind, ID: c.ID, Name: c.Name()}, true
}

// RefreshTarget implements core.Refreshable: the competition the selected
// card's numbers come from.
func (m *Model) RefreshTarget() (string, bool) {
	c, ok := m.Selected()
	if !ok || c.Missing || c.Competition.ID == "" {
		return "", false
	}
	return c.Competition.ID, true
}

type cardsLoadedMsg struct{ cards []Card }

// kindOrder is the dashboard's fixed group order.
var kindOrder = map[models.FavoriteKind]int{
	models.FavoriteCompetition: 0,
	models.FavoriteTeam:        1,
	models.FavoritePlayer:      2,
}

// load builds every card. A favourite whose entity is gone becomes a
// Missing card rather than an error.
func (m *Model) load() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		favs, err := deps.Repos.ListFavorites(deps.Ctx)
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		// ListFavorites is oldest first; a stable sort keeps that order
		// within each group.
		sort.SliceStable(favs, func(i, j int) bool { return kindOrder[favs[i].Kind] < kindOrder[favs[j].Kind] })

		cards := make([]Card, 0, len(favs))
		for _, f := range favs {
			c, err := loadCard(deps, f)
			if errors.Is(err, storage.ErrNotFound) {
				c = Card{Kind: f.Kind, ID: f.ID, Missing: true}
			} else if err != nil {
				return core.ErrMsg{Err: err}
			}
			cards = append(cards, c)
		}
		return cardsLoadedMsg{cards: cards}
	}
}

func loadCard(deps core.Deps, f models.Favorite) (Card, error) {
	switch f.Kind {
	case models.FavoriteCompetition:
		return loadLeague(deps, f.ID)
	case models.FavoriteTeam:
		return loadTeam(deps, f.ID)
	default:
		return loadPlayer(deps, f.ID)
	}
}

func loadLeague(deps core.Deps, id string) (Card, error) {
	c := Card{Kind: models.FavoriteCompetition, ID: id}
	comp, err := deps.Repos.GetCompetition(deps.Ctx, id)
	if err != nil {
		return c, err
	}
	c.Competition = comp
	seasons, err := deps.Repos.ListSeasons(deps.Ctx, id)
	if err != nil || len(seasons) == 0 {
		return c, err
	}
	c.Season = seasons[0]
	stats, err := deps.Repos.Standings(deps.Ctx, id, c.Season.ID)
	if err != nil {
		return c, err
	}
	ids := make([]string, 0, len(stats))
	for _, s := range stats {
		ids = append(ids, s.TeamID)
	}
	teams, err := deps.Repos.GetTeams(deps.Ctx, ids)
	if err != nil {
		return c, err
	}
	for i, s := range stats {
		team, ok := teams[s.TeamID]
		if !ok {
			team = models.Team{ID: s.TeamID, Name: s.TeamID}
		}
		row := StandingRow{Position: s.Position, Team: team, Points: s.Points, Played: s.MatchesPlayed}
		switch {
		case i < tableRows:
			c.Table = append(c.Table, row)
		case deps.Favorites.Has(models.FavoriteTeam, team.ID):
			c.Starred = append(c.Starred, row)
		}
	}
	c.LastSync = lastSync(deps, id, c.Season.ID)
	return c, nil
}

func loadTeam(deps core.Deps, id string) (Card, error) {
	c := Card{Kind: models.FavoriteTeam, ID: id}
	team, err := deps.Repos.GetTeam(deps.Ctx, id)
	if err != nil {
		return c, err
	}
	c.Team = team
	history, err := deps.Repos.TeamHistory(deps.Ctx, id)
	if err != nil || len(history) == 0 {
		return c, err
	}
	seasonIDs := make([]string, 0, len(history))
	for _, h := range history {
		seasonIDs = append(seasonIDs, h.SeasonID)
	}
	seasons, err := deps.Repos.GetSeasons(deps.Ctx, seasonIDs)
	if err != nil {
		return c, err
	}
	// The latest season, and within it the league (a row with a table
	// position) over a cup: "3rd in the Premier League" is the headline,
	// not a Champions League group.
	best := -1
	for i, h := range history {
		if best < 0 {
			best = i
			continue
		}
		bl, hl := seasons[history[best].SeasonID].Label, seasons[h.SeasonID].Label
		switch {
		case hl > bl:
			best = i
		case hl == bl && history[best].Position == nil && h.Position != nil:
			best = i
		}
	}
	stats := history[best]
	c.TeamStats = &stats
	c.Season = seasons[stats.SeasonID]
	if comp, err := deps.Repos.GetCompetition(deps.Ctx, stats.CompetitionID); err == nil {
		c.Competition = comp
	} else {
		c.Competition = models.Competition{ID: stats.CompetitionID, Name: stats.CompetitionID}
	}
	c.LastSync = lastSync(deps, stats.CompetitionID, stats.SeasonID)
	return c, nil
}

func loadPlayer(deps core.Deps, id string) (Card, error) {
	c := Card{Kind: models.FavoritePlayer, ID: id}
	player, err := deps.Repos.GetPlayer(deps.Ctx, id)
	if err != nil {
		return c, err
	}
	c.Player = player
	history, err := deps.Repos.PlayerHistory(deps.Ctx, id)
	if err != nil || len(history) == 0 {
		return c, err
	}
	seasonIDs := make([]string, 0, len(history))
	for _, h := range history {
		seasonIDs = append(seasonIDs, h.SeasonID)
	}
	seasons, err := deps.Repos.GetSeasons(deps.Ctx, seasonIDs)
	if err != nil {
		return c, err
	}
	// The latest season, and within it the line with the most minutes:
	// the club and competition they mostly played in.
	best := 0
	for i, h := range history {
		bl, hl := seasons[history[best].SeasonID].Label, seasons[h.SeasonID].Label
		if hl > bl || (hl == bl && h.Minutes > history[best].Minutes) {
			best = i
		}
	}
	stats := history[best]
	c.PlayerStats = &stats
	c.Season = seasons[stats.SeasonID]
	if team, err := deps.Repos.GetTeam(deps.Ctx, stats.TeamID); err == nil {
		c.Team = team
	} else {
		c.Team = models.Team{ID: stats.TeamID, Name: stats.TeamID}
	}
	if comp, err := deps.Repos.GetCompetition(deps.Ctx, stats.CompetitionID); err == nil {
		c.Competition = comp
	} else {
		c.Competition = models.Competition{ID: stats.CompetitionID, Name: stats.CompetitionID}
	}
	c.LastSync = lastSync(deps, stats.CompetitionID, stats.SeasonID)
	return c, nil
}

// lastSync is the latest refresh of one competition-season, nil when it
// was never refreshed or the log can't be read — freshness is a nicety
// and never fails a card.
func lastSync(deps core.Deps, compID, seasonID string) *models.SyncRun {
	run, err := deps.Repos.LatestSyncRun(deps.Ctx, compID, seasonID)
	if err != nil {
		return nil
	}
	return &run
}
