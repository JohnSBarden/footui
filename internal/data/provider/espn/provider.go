package espn

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	datasync "github.com/JohnSBarden/footui/internal/data/sync"
)

// Provider implements the sync layer's Source for ESPN competitions.
type Provider struct {
	client *Client
	now    func() time.Time
	// limit is how many matches one scoreboard request asks for.
	limit int

	mu      sync.Mutex
	bundles map[string]*bundle
}

// New returns a provider against the real API.
func New() *Provider { return newWithClient(NewClient()) }

// NewAt returns a provider against another host serving ESPN's paths (a
// test server), unpaced and with its own rate-limit breaker, so another
// package's tests can count the requests a refresh makes.
func NewAt(baseURL string) *Provider {
	return newWithClient(NewClient().withBaseURLs(baseURL+"/site", baseURL+"/core"))
}

func newWithClient(c *Client) *Provider {
	return &Provider{client: c, now: time.Now, limit: scoreboardLimit, bundles: map[string]*bundle{}}
}

// OnRetry reports each retried request, for progress output.
func (p *Provider) OnRetry(fn func(attempt, of int, err error)) *Provider {
	p.client.OnRetry = fn
	return p
}

// bundle is everything one competition-season refresh needs. The sync
// layer asks for teams, standings, players, and stats separately; they
// are fetched together once and served from here.
type bundle struct {
	teams     []models.Team
	standings []models.TeamSeasonStats
	players   []models.Player
	lines     []models.PlayerSeasonStats
	colors    []models.TeamColors
}

// Teams implements provider.TeamsProvider.
func (p *Provider) Teams(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	b, err := p.load(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return b.teams, nil
}

// Standings implements provider.StandingsProvider.
func (p *Provider) Standings(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	b, err := p.load(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return b.standings, nil
}

// Players implements provider.LeaguePlayersProvider.
func (p *Provider) Players(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Player, error) {
	b, err := p.load(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return b.players, nil
}

// AllRosterStats implements provider.LeaguePlayersProvider.
func (p *Provider) AllRosterStats(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	b, err := p.load(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return b.lines, nil
}

// TeamColors returns the brand colours ESPN has for the competition's
// clubs. They fill in for clubs nobody has curated colours for.
func (p *Provider) TeamColors(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamColors, error) {
	b, err := p.load(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return b.colors, nil
}

func (p *Provider) load(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) (*bundle, error) {
	key := comp.CompetitionID + "/" + season.Label
	p.mu.Lock()
	b, ok := p.bundles[key]
	p.mu.Unlock()
	if ok {
		return b, nil
	}
	b, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.bundles[key] = b
	p.mu.Unlock()
	return b, nil
}

// competitionFor recovers what mapping needs to know about a competition
// (its country and gender) from the coverage registry: the sync layer
// passes only an ID and a name.
func competitionFor(ref provider.CompetitionRef) models.Competition {
	if c, ok := datasync.FindCompetition(ref.CompetitionID); ok {
		return c
	}
	return models.Competition{ID: ref.CompetitionID, Name: ref.Name, Gender: "men"}
}

// ErrNoTable means ESPN has no standings for that season: before the
// competition existed, or a season it never published a table for.
var ErrNoTable = fmt.Errorf("espn: %w", provider.ErrNoTable)

func (p *Provider) fetch(ctx context.Context, ref provider.CompetitionRef, season provider.SeasonRef) (*bundle, error) {
	slug, ok := leagueSlugs[ref.CompetitionID]
	if !ok {
		return nil, fmt.Errorf("espn: competition %q is not covered", ref.CompetitionID)
	}
	comp := competitionFor(ref)
	now := p.now().UTC()
	year := season.Label

	var st standingsResponse
	if err := p.client.site(ctx, fmt.Sprintf("/v2/sports/soccer/%s/standings?season=%s", slug, year), &st); err != nil {
		return nil, fmt.Errorf("espn: standings %s %s: %w", slug, year, err)
	}
	b := &bundle{}
	b.teams, b.standings = mapStandings(st, comp, season.SeasonID, now)
	if len(b.standings) == 0 {
		return nil, fmt.Errorf("%w (%s %s)", ErrNoTable, slug, year)
	}
	// A tournament's table is the whole story: national squads change
	// from one match to the next, and footui keeps no player stats for
	// them, so there are no rosters to fetch.
	if comp.International() {
		return b, nil
	}

	// Colors are decoration: a failure here costs nothing but theming.
	var tl teamsResponse
	if err := p.client.site(ctx, fmt.Sprintf("/site/v2/sports/soccer/%s/teams", slug), &tl); err == nil {
		b.colors = mapColors(tl)
	}

	rosterFailures := 0
	for i := range b.standings {
		row := &b.standings[i]
		espnID := row.TeamID[len(idPrefix):]
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var ro rosterResponse
		err := p.client.site(ctx, fmt.Sprintf("/site/v2/sports/soccer/%s/teams/%s/roster?season=%s", slug, espnID, year), &ro)
		if err != nil {
			rosterFailures++
		} else {
			players, lines := mapRoster(ro, comp, espnID, season.SeasonID, now)
			b.players = append(b.players, players...)
			b.lines = append(b.lines, lines...)
		}

		var ts categories
		if err := p.client.core(ctx, fmt.Sprintf("/%s/seasons/%s/types/1/teams/%s/statistics", slug, year, espnID), &ts); err == nil {
			applyTeamStats(row, ts)
		}
	}
	// One club's roster missing leaves a hole; every roster missing means
	// something is wrong with the whole request, and a table with no
	// squads behind it shouldn't be reported as a successful refresh.
	if rosterFailures == len(b.standings) {
		return nil, fmt.Errorf("espn: no rosters available for %s %s", slug, year)
	}
	return b, nil
}
