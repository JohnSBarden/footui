package espn

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
)

// HasDetail reports whether ESPN has player season detail for a
// competition.
func (p *Provider) HasDetail(competitionID string) bool {
	_, ok := slugFor(competitionID)
	return ok
}

// AthleteSeason fetches one player's season in one competition from the
// core API: the detail a position is judged on. keeper says whether the
// player keeps goal; ESPN reports the club's saves against outfield
// players too, so for anyone else they're dropped. The IDs are left for
// the caller to fill in.
//
// ESPN only records the defensive stats for recent seasons and reports
// zeros for older ones, so an outfield season with real minutes and not
// a single tackle, interception or clearance is "not recorded", as is a
// pass rate of zero.
func (p *Provider) AthleteSeason(ctx context.Context, competitionID string, year int, athleteID string, keeper bool) (models.PlayerSeasonDetail, error) {
	slug, ok := slugFor(competitionID)
	if !ok {
		return models.PlayerSeasonDetail{}, fmt.Errorf("espn: no player detail for %q", competitionID)
	}
	var resp categories
	path := fmt.Sprintf("/%s/seasons/%d/types/1/athletes/%s/statistics", slug, year, athleteID)
	if err := p.client.core(ctx, path, &resp); err != nil {
		return models.PlayerSeasonDetail{}, err
	}
	st := resp.all()
	count := func(name string) *int {
		v, ok := st.get(name)
		if !ok {
			return nil
		}
		n := int(v)
		return &n
	}

	d := models.PlayerSeasonDetail{
		Appearances:   count("appearances"),
		Minutes:       count("minutes"),
		CleanSheets:   count("cleanSheet"),
		GoalsConceded: count("goalsConceded"),
		Tackles:       count("totalTackles"),
		Interceptions: count("interceptions"),
		Clearances:    count("totalClearance"),
		ShotsOnTarget: count("shotsOnTarget"),
	}
	if keeper {
		d.Saves = count("saves")
	}
	if v, ok := st.get("passPct"); ok && v > 0 {
		d.PassPct = &v
	}
	if !keeper && (implausibleDefence(d) ||
		d.Minutes != nil && *d.Minutes >= 90 && zero(d.Tackles) && zero(d.Interceptions) && zero(d.Clearances)) {
		d.Tackles, d.Interceptions, d.Clearances = nil, nil, nil
	}
	return d, nil
}

func zero(n *int) bool { return n == nil || *n == 0 }

// implausibleDefence reports a season whose defensive numbers are a token
// handful rather than a record: under half a tackle, interception or
// clearance per 90 over 900 minutes or more. Even a pure striker manages
// nearly twice that; ESPN's older seasons often carry less.
func implausibleDefence(d models.PlayerSeasonDetail) bool {
	if d.Minutes == nil || *d.Minutes < 900 {
		return false
	}
	n := 0
	for _, p := range []*int{d.Tackles, d.Interceptions, d.Clearances} {
		if p != nil {
			n += *p
		}
	}
	return float64(n)*90/float64(*d.Minutes) < 0.5
}

// Squad lists a club's roster for a season: each player's ESPN ID, name
// and position, for matching cached players to ESPN's.
func (p *Provider) Squad(ctx context.Context, competitionID string, year int, espnTeamID string) ([]provider.SquadMember, error) {
	slug, ok := slugFor(competitionID)
	if !ok {
		return nil, fmt.Errorf("espn: no rosters for %q", competitionID)
	}
	var ro rosterResponse
	path := fmt.Sprintf("/site/v2/sports/soccer/%s/teams/%s/roster?season=%s", slug, espnTeamID, strconv.Itoa(year))
	if err := p.client.site(ctx, path, &ro); err != nil {
		return nil, err
	}
	out := make([]provider.SquadMember, 0, len(ro.Athletes))
	for _, a := range ro.Athletes {
		out = append(out, provider.SquadMember{ID: a.ID, Name: a.DisplayName, Position: a.Position.Abbreviation})
	}
	return out, nil
}
