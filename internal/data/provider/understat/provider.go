package understat

import (
	"context"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

// Provider serves footui's provider contracts from understat.com.
type Provider struct {
	client *Client
}

// New constructs a Provider with a 1-second gap between requests.
func New() *Provider {
	return &Provider{client: NewClient(1 * time.Second)}
}

// OnRetry registers fn to be told before each retry, so a slow refresh
// doesn't look like a hang.
func (p *Provider) OnRetry(fn func(attempt, of int, err error)) *Provider {
	p.client.OnRetry = fn
	return p
}

var _ provider.StandingsProvider = (*Provider)(nil)
var _ provider.TeamsProvider = (*Provider)(nil)
var _ provider.LeaguePlayersProvider = (*Provider)(nil)
var _ provider.SeasonMatchesProvider = (*Provider)(nil)

func (p *Provider) fetch(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) (*leagueData, error) {
	leagueKey, ok := leagueKeys[comp.CompetitionID]
	if !ok {
		return nil, fmt.Errorf("understat: competition %q is not covered (supported: %v)", comp.CompetitionID, leagueKeys)
	}
	data, err := p.client.fetchLeagueData(ctx, leagueKey, season.Label)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Standings implements provider.StandingsProvider.
func (p *Provider) Standings(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapStandings(data, comp.CompetitionID, season.SeasonID), nil
}

// Teams implements provider.TeamsProvider.
func (p *Provider) Teams(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapTeamEntities(data, comp.CompetitionID), nil
}

// AllRosterStats implements provider.LeaguePlayersProvider.
func (p *Provider) AllRosterStats(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	rows := mapPlayers(data, comp.CompetitionID, season.SeasonID)
	return p.splitMovers(ctx, data, comp.CompetitionID, season.Label, rows), nil
}

// SeasonMatches implements provider.SeasonMatchesProvider; it costs no
// extra request.
func (p *Provider) SeasonMatches(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Match, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapMatches(data, comp.CompetitionID, season.SeasonID, time.Now().UTC()), nil
}

// Players implements provider.LeaguePlayersProvider.
func (p *Provider) Players(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Player, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapPlayerEntities(data), nil
}
