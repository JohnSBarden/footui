package espn

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
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

// AthleteSeason fetches one player's season detail; the caller fills in
// the IDs. Saves are kept only for a keeper, since ESPN reports the
// club's saves against outfield players too. ESPN reports zeros for
// older seasons' defensive stats, so those become "not recorded".
func (p *Provider) AthleteSeason(ctx context.Context, competitionID string, year int, athleteID string, keeper bool) (models.PlayerSeasonDetail, error) {
	slug, ok := slugFor(competitionID)
	if !ok {
		return models.PlayerSeasonDetail{}, fmt.Errorf("espn: no player detail for %q", competitionID)
	}
	var resp categories
	path := fmt.Sprintf("/%s/seasons/%d/types/1/athletes/%s/statistics", slug, year, url.PathEscape(athleteID))
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

// implausibleDefence reports under 0.5 defensive actions per 90 over 900+
// minutes: even a pure striker manages nearly twice that.
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

// careerLog names each league-season only by URL.
type careerLog struct {
	Entries []struct {
		Season struct {
			Ref string `json:"$ref"`
		} `json:"season"`
	} `json:"entries"`
}

// leagueSeasonRef matches ".../leagues/esp.1/seasons/2014?lang=en".
var leagueSeasonRef = regexp.MustCompile(`/leagues/([^/?]+)/seasons/(\d{4})`)

// AthleteCareer lists the player's league-seasons that footui covers,
// oldest first, deduplicated (a mid-season move lists one twice).
func (p *Provider) AthleteCareer(ctx context.Context, athleteID string) ([]provider.CareerSeason, error) {
	var log careerLog
	if err := p.client.core(ctx, "/all/athletes/"+url.PathEscape(athleteID)+"/statisticslog", &log); err != nil {
		return nil, err
	}
	bySlug := make(map[string]string, len(leagueSlugs)+len(statSlugs))
	for id, slug := range leagueSlugs {
		bySlug[slug] = id
	}
	for id, slug := range statSlugs {
		bySlug[slug] = id
	}
	var out []provider.CareerSeason
	seen := map[provider.CareerSeason]bool{}
	for _, e := range log.Entries {
		m := leagueSeasonRef.FindStringSubmatch(e.Season.Ref)
		if m == nil {
			continue
		}
		id, ok := bySlug[m[1]]
		if !ok {
			continue
		}
		year, _ := strconv.Atoi(m[2])
		cs := provider.CareerSeason{CompetitionID: id, Year: year}
		if !seen[cs] {
			seen[cs] = true
			out = append(out, cs)
		}
	}
	return out, nil
}

// Squad lists a club's roster for a season, for matching cached players.
func (p *Provider) Squad(ctx context.Context, competitionID string, year int, espnTeamID string) ([]provider.SquadMember, error) {
	slug, ok := slugFor(competitionID)
	if !ok {
		return nil, fmt.Errorf("espn: no rosters for %q", competitionID)
	}
	var ro rosterResponse
	path := fmt.Sprintf("/site/v2/sports/soccer/%s/teams/%s/roster?season=%s", slug, url.PathEscape(espnTeamID), strconv.Itoa(year))
	if err := p.client.site(ctx, path, &ro); err != nil {
		return nil, err
	}
	out := make([]provider.SquadMember, 0, len(ro.Athletes))
	for _, a := range ro.Athletes {
		out = append(out, provider.SquadMember{ID: a.ID, Name: a.DisplayName, Position: squadPosition(a.Position.Abbreviation)})
	}
	return out, nil
}

// squadPosition maps to provider.SquadMember's letters, "" if unknown.
func squadPosition(abbr string) string {
	switch abbr {
	case "G", "GK":
		return "G"
	case "D", "M", "F":
		return abbr
	}
	return ""
}
