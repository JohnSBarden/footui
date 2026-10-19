// Package dashboard is the landing page once anything is favourited: a
// card per favourite, each showing where it stands this season, and each
// drilling into the screen Browse would open.
//
// Cards run in a fixed order — leagues, tournaments, clubs, nations,
// players — and within each kind by when they were starred, flowing
// together into columns; each card's badge says what kind it is.
// Search is one key away (/), so the dashboard replaces the bare search
// box without hiding it.
package dashboard

import (
	"errors"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/core"
	"github.com/JohnSBarden/footui/internal/tui/keys"
	"github.com/JohnSBarden/footui/internal/tui/scorestrip"
)

// Title is the dashboard's breadcrumb title.
const Title = "Dashboard"

// tableRows is how many standings rows a tournament card shows;
// leagueRows how many a league's card, twice the size of the others,
// does.
const (
	tableRows  = 5
	leagueRows = 15
)

// StandingRow is one line of a league card's table.
type StandingRow struct {
	Position *int
	Team     models.Team
	Points   int
	Played   int
	Wins     int
	Draws    int
	Losses   int
	GoalDiff int
}

func standingRow(s models.TeamSeasonStats, team models.Team) StandingRow {
	return StandingRow{Position: s.Position, Team: team, Points: s.Points, Played: s.MatchesPlayed,
		Wins: s.Wins, Draws: s.Draws, Losses: s.Losses, GoalDiff: s.GoalDiff}
}

// Group is a card's section of the dashboard, in display order.
type Group int

// Dashboard groups.
const (
	GroupLeagues Group = iota
	GroupTournaments
	GroupClubs
	GroupNations
	GroupPlayers
)

// Card is one favourite's card. Kind and Group select which of the
// fields below mean anything.
type Card struct {
	Kind  models.FavoriteKind
	Group Group
	ID    string
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

	// Club cards. Summary (below) carries the club's form over its last
	// games in every competition.
	Team      models.Team
	TeamStats *models.TeamSeasonStats

	// Player cards. Team is the club the line is for.
	Player      models.Player
	PlayerStats *models.PlayerSeasonStats
	// Position is the player's group; Detail that season's clean sheets,
	// tackles and so on (nil until fetched); XGA a keeper's estimated
	// expected goals against; Standouts the season's numbers unusual for
	// the position.
	Position  playerstats.Group
	Detail    *models.PlayerSeasonDetail
	XGA       *float64
	Standouts []playerstats.Standout

	// Tournament cards show a group's table (GroupLabel, in Table) during
	// the group stage, and the latest round's results after it.
	GroupLabel string
	RoundLabel string
	Results    []models.Match

	// Nation cards (Team is the nation), under its saved filter: the last
	// few results, the next fixture, and its record.
	Filter  models.MatchFilter
	Recent  []models.Match
	Next    *models.Match
	Summary models.MatchSummary
	// Updated is when the newest of a nation's matches was fetched.
	Updated time.Time

	// Teams and Comps name the sides and competitions in match lines.
	Teams map[string]models.Team
	Comps map[string]models.Competition

	// Colors are the brand colours of the clubs on the card, keyed by
	// team ID, for its dots and scarf. A club without any is absent.
	Colors map[string]models.TeamColors
	// LeagueSize is how many teams are in a club card's table, so the
	// card can tell the bottom three; zero when unknown.
	LeagueSize int
}

// Name is the card's heading.
func (c Card) Name() string {
	switch {
	case c.Missing:
		return c.ID
	case c.Kind == models.FavoriteCompetition:
		return c.Competition.Name
	case c.Kind == models.FavoriteTeam:
		return c.Team.Label()
	default:
		return c.Player.Name
	}
}

// Model is the dashboard screen.
type Model struct {
	deps core.Deps

	cards  []Card
	cursor int
	// strip is the scores along the top: the favourites' latest games
	// and next ones.
	strip scorestrip.Strip

	// active and pending are what is being refreshed and waiting to be,
	// keyed like core.RefreshRequest.Key, so a card can say its numbers
	// are on their way.
	active  map[string]bool
	pending map[string]bool

	width  int
	height int

	loading bool
	err     error
}

// New constructs the dashboard. Cards load in Init.
func New(deps core.Deps) *Model {
	return &Model{deps: deps, loading: true, width: 100, height: 30, active: map[string]bool{}, pending: map[string]bool{}}
}

// Title implements core.Screen.
func (m *Model) Title() string { return Title }

// Keys implements core.Screen.
func (m *Model) Keys() core.KeyMap {
	return core.KeyMap{
		Short: []key.Binding{keys.MoveCard, keys.Open, keys.Scores},
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
	if !ok || c.Missing || c.Competition.ID == "" || c.Group == GroupNations {
		return "", false
	}
	return c.Competition.ID, true
}

// RefreshRequest implements core.RefreshRequester: a nation card's
// matches.
func (m *Model) RefreshRequest() (core.RefreshRequest, bool) {
	c, ok := m.Selected()
	if !ok || c.Missing || c.Group != GroupNations {
		return core.RefreshRequest{}, false
	}
	return core.RefreshRequest{TeamID: c.Team.ID, Name: c.Team.Label()}, true
}

type cardsLoadedMsg struct {
	cards []Card
	strip scorestrip.Strip
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
		cards := make([]Card, 0, len(favs))
		for _, f := range favs {
			c, err := loadCard(deps, f)
			if errors.Is(err, storage.ErrNotFound) {
				c = Card{Kind: f.Kind, ID: f.ID, Missing: true, Group: missingGroup(f.Kind)}
			} else if err != nil {
				return core.ErrMsg{Err: err}
			}
			cards = append(cards, c)
		}
		// ListFavorites is oldest first; a stable sort keeps that order
		// within each group.
		sort.SliceStable(cards, func(i, j int) bool { return cards[i].Group < cards[j].Group })
		colors, err := deps.Repos.TeamColorsFor(deps.Ctx, clubIDs(cards))
		if err != nil {
			return core.ErrMsg{Err: err}
		}
		for i := range cards {
			cards[i].Colors = colors
		}
		return cardsLoadedMsg{cards: cards, strip: loadStrip(deps, cards)}
	}
}

// loadStrip is the scores along the top: each starred club's and
// nation's latest games and next, a starred player's club's, and each
// starred league's and tournament's latest round and next. It is a
// nicety, so a failure leaves it empty rather than failing the screen.
func loadStrip(deps core.Deps, cards []Card) scorestrip.Strip {
	t := now()
	var teamIDs []string
	var matches []models.Match
	for _, c := range cards {
		switch {
		case c.Missing:
		case c.Group == GroupLeagues || c.Group == GroupTournaments:
			if c.Season.ID == "" {
				continue
			}
			ms, err := scorestrip.ForSeason(deps, c.Competition.ID, c.Season.ID, t)
			if err != nil {
				return scorestrip.Strip{}
			}
			matches = append(matches, ms...)
		case c.Team.ID != "":
			teamIDs = append(teamIDs, c.Team.ID)
		}
	}
	ms, err := scorestrip.ForTeams(deps, teamIDs, t)
	if err != nil {
		return scorestrip.Strip{}
	}
	strip, err := scorestrip.Build(deps, append(matches, ms...), t)
	if err != nil {
		return scorestrip.Strip{}
	}
	strip.ShowComp = true
	return strip
}

// clubIDs are the clubs whose colours the cards draw: each club card's
// own, a player's club, and every club on a league card.
func clubIDs(cards []Card) []string {
	var ids []string
	for _, c := range cards {
		if c.Missing {
			continue
		}
		switch c.Group {
		case GroupClubs, GroupPlayers:
			ids = append(ids, c.Team.ID)
		case GroupLeagues:
			for _, r := range c.Table {
				ids = append(ids, r.Team.ID)
			}
			for _, r := range c.Starred {
				ids = append(ids, r.Team.ID)
			}
		}
	}
	return ids
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

// missingGroup is where a card whose entity isn't cached goes.
func missingGroup(k models.FavoriteKind) Group {
	switch k {
	case models.FavoriteCompetition:
		return GroupLeagues
	case models.FavoriteTeam:
		return GroupClubs
	}
	return GroupPlayers
}

func loadLeague(deps core.Deps, id string) (Card, error) {
	c := Card{Kind: models.FavoriteCompetition, ID: id, Group: GroupLeagues}
	comp, err := deps.Repos.GetCompetition(deps.Ctx, id)
	if err != nil {
		return c, err
	}
	if comp.International() {
		return loadTournament(deps, comp)
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
		row := standingRow(s, team)
		switch {
		case i < leagueRows:
			c.Table = append(c.Table, row)
		case deps.Favorites.Has(models.FavoriteTeam, team.ID):
			c.Starred = append(c.Starred, row)
		}
	}
	c.LastSync = lastSync(deps, id, c.Season.ID)
	return c, nil
}

func loadTeam(deps core.Deps, id string) (Card, error) {
	c := Card{Kind: models.FavoriteTeam, ID: id, Group: GroupClubs}
	team, err := deps.Repos.GetTeam(deps.Ctx, id)
	if err != nil {
		return c, err
	}
	if team.National {
		return loadNation(deps, team)
	}
	c.Team = team
	// The club's form, from its last games in every competition.
	if recent, err := deps.Repos.RecentMatches(deps.Ctx, id, 5); err == nil {
		c.Summary = models.SummarizeMatches(id, recent)
	}
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
	if stats.Position != nil {
		if table, err := deps.Repos.Standings(deps.Ctx, stats.CompetitionID, stats.SeasonID); err == nil {
			c.LeagueSize = len(table)
		}
	}
	return c, nil
}

func loadPlayer(deps core.Deps, id string) (Card, error) {
	c := Card{Kind: models.FavoritePlayer, ID: id, Group: GroupPlayers}
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
	playerDetail(deps, &c, history)
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
