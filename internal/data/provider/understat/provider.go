package understat

import (
	"context"
	"fmt"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

// Provider implements the StandingsProvider, XGProvider, and
// PlayerStatsProvider contracts (internal/data/provider) against
// understat.com. It does not implement TeamStatsProvider in the "richer
// stat line" sense (possession/fouls/cards/shots) — understat.com doesn't
// have that data — so TeamSeasonStats returns the same standings-shaped
// row Standings does, with those fields left nil.
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
var _ provider.XGProvider = (*Provider)(nil)
var _ provider.PlayerStatsProvider = (*Provider)(nil)
var _ provider.TeamsProvider = (*Provider)(nil)
var _ provider.LeaguePlayersProvider = (*Provider)(nil)

// Covers reports whether this provider has a mapping for the given
// competition ID at all — the sync layer uses this to decide routing
// without needing to know understat.com's internal league keys.
func Covers(competitionID string) bool {
	_, ok := leagueKeys[competitionID]
	return ok
}

func (p *Provider) fetch(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) (*leagueData, string, error) {
	leagueKey, ok := leagueKeys[comp.CompetitionID]
	if !ok {
		return nil, "", fmt.Errorf("understat: competition %q is not covered (supported: %v)", comp.CompetitionID, leagueKeys)
	}
	data, err := p.client.fetchLeagueData(ctx, leagueKey, season.Label)
	if err != nil {
		return nil, "", err
	}
	return data, leagueKey, nil
}

// Standings implements provider.StandingsProvider.
func (p *Provider) Standings(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.TeamSeasonStats, error) {
	data, _, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapStandings(data, comp.CompetitionID, season.SeasonID), nil
}

// Teams returns the Team identity rows referenced by this competition/season
// — needed alongside Standings since TeamSeasonStats only carries team IDs,
// not full Team records.
func (p *Provider) Teams(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Team, error) {
	data, _, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapTeamEntities(data, comp.CompetitionID), nil
}

// TeamXG implements provider.XGProvider.
func (p *Provider) TeamXG(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef, team provider.TeamRef) (float64, float64, error) {
	stats, err := p.Standings(ctx, comp, season)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range stats {
		if s.TeamID == team.TeamID {
			var xg, xga float64
			if s.XG != nil {
				xg = *s.XG
			}
			if s.XGA != nil {
				xga = *s.XGA
			}
			return xg, xga, nil
		}
	}
	return 0, 0, fmt.Errorf("understat: team %q not found in %s %s", team.TeamID, comp.CompetitionID, season.Label)
}

// PlayerXG implements provider.XGProvider.
func (p *Provider) PlayerXG(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef, player provider.PlayerRef) (float64, float64, error) {
	data, _, err := p.fetch(ctx, comp, season)
	if err != nil {
		return 0, 0, err
	}
	for _, ps := range mapPlayers(data, comp.CompetitionID, season.SeasonID) {
		if ps.PlayerID == player.PlayerID {
			var xg, xa float64
			if ps.XG != nil {
				xg = *ps.XG
			}
			if ps.XA != nil {
				xa = *ps.XA
			}
			return xg, xa, nil
		}
	}
	return 0, 0, fmt.Errorf("understat: player %q not found in %s %s", player.PlayerID, comp.CompetitionID, season.Label)
}

// RosterStats implements provider.PlayerStatsProvider. team is currently
// ignored: understat.com's payload returns every player in the league in
// one call, already tagged with team_title, so filtering happens here
// rather than via a per-team request.
func (p *Provider) RosterStats(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef, team provider.TeamRef) ([]models.PlayerSeasonStats, error) {
	data, _, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	all := mapPlayers(data, comp.CompetitionID, season.SeasonID)
	out := make([]models.PlayerSeasonStats, 0, len(all))
	for _, ps := range all {
		if ps.TeamID == team.TeamID {
			out = append(out, ps)
		}
	}
	return out, nil
}

// AllRosterStats returns every player's season stats across the whole
// league/season in one call — a sync convenience on top of RosterStats,
// which filters to one team, so a full-league sync doesn't need one
// network round trip per team (the underlying payload is cached after the
// first fetch either way).
func (p *Provider) AllRosterStats(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.PlayerSeasonStats, error) {
	data, _, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapPlayers(data, comp.CompetitionID, season.SeasonID), nil
}

// Players returns the Player identity rows (name/position/gender) for an
// entire league/season payload, for upserting alongside RosterStats.
func (p *Provider) Players(ctx context.Context, comp provider.CompetitionRef, season provider.SeasonRef) ([]models.Player, error) {
	data, _, err := p.fetch(ctx, comp, season)
	if err != nil {
		return nil, err
	}
	return mapPlayerEntities(data), nil
}
