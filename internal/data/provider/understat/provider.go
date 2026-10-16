package understat

import (
	"context"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

// Provider serves footui's provider contracts from understat.com, answering
// every one of them from a single cached league-season payload.
type Provider struct {
	client *Client
}

// New constructs a Provider with a default 1-second minimum gap between
// outbound requests.
func New() *Provider {
	return &Provider{client: NewClient(1 * time.Second)}
}

// OnRetry registers fn to be told each time a failed request is about to
// be retried, with the attempt that failed, the total allowed, and why.
// Callers that report progress to a user should set it: a retry cycle
// against an unresponsive host otherwise sits silent for the length of
// every timeout it waits through.
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

// Teams returns the Team identity rows referenced by this competition/season
// — needed alongside Standings since TeamSeasonStats only carries team IDs,
// not full Team records.
func (p *Provider) Teams(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapTeamEntities(data, comp.CompetitionID), nil
}

// AllRosterStats returns every player's season stats across the whole
// league-season.
func (p *Provider) AllRosterStats(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapPlayers(data, comp.CompetitionID, season.SeasonID), nil
}

// SeasonMatches implements provider.SeasonMatchesProvider from the same
// payload as everything else, so a league's matches cost no request of
// their own.
func (p *Provider) SeasonMatches(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Match, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapMatches(data, comp.CompetitionID, season.SeasonID, time.Now().UTC()), nil
}

// Players returns the Player identity rows (name/position/gender) for an
// entire league/season payload, for upserting alongside AllRosterStats.
func (p *Provider) Players(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Player, error) {
	data, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapPlayerEntities(data), nil
}
