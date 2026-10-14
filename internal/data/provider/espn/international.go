package espn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/provider"
	footysync "github.com/JohnSBarden/footui/internal/data/sync"
	"github.com/JohnSBarden/footui/internal/textfold"
)

// International football is fetched a competition-year at a time: one
// scoreboard request returns every match of, say, the 2025 friendlies —
// every nation's — with each side's possession, shots, corners and fouls.
// Per-match summaries carry more, but at 440 KB each they are far too
// heavy to fetch for hundreds of matches.

// scoreboardLimit is how many events one scoreboard request asks for.
// ESPN's default is small and truncates silently; the busiest year of
// friendlies is about 300 matches.
const scoreboardLimit = 1000

// --- wire types ---

type scoreboardResponse struct {
	Events []scoreboardEvent `json:"events"`
}

type scoreboardEvent struct {
	ID     string `json:"id"`
	Date   string `json:"date"`
	Season struct {
		Year int    `json:"year"`
		Slug string `json:"slug"`
	} `json:"season"`
	Competitions []struct {
		Date   string `json:"date"`
		Status struct {
			Type struct {
				Name      string `json:"name"`
				State     string `json:"state"`
				Completed bool   `json:"completed"`
			} `json:"type"`
		} `json:"status"`
		NeutralSite *bool `json:"neutralSite"`
		Venue       struct {
			Address struct {
				Country string `json:"country"`
			} `json:"address"`
		} `json:"venue"`
		Group *struct {
			Name string `json:"name"`
		} `json:"group"`
		Competitors []scoreboardCompetitor `json:"competitors"`
	} `json:"competitions"`
}

type scoreboardCompetitor struct {
	HomeAway      string   `json:"homeAway"`
	Score         flexNum  `json:"score"`
	ShootoutScore *flexNum `json:"shootoutScore"`
	Team          struct {
		ID               string `json:"id"`
		DisplayName      string `json:"displayName"`
		ShortDisplayName string `json:"shortDisplayName"`
		Location         string `json:"location"`
	} `json:"team"`
	Statistics []struct {
		Name         string `json:"name"`
		DisplayValue string `json:"displayValue"`
	} `json:"statistics"`
}

// flexNum is a number ESPN sends either as a JSON number or as a string
// ("2"); Valid is false when it was absent or unreadable.
type flexNum struct {
	Value float64
	Valid bool
}

func (n *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // an unreadable score is a missing one, not a failed fetch
	}
	n.Value, n.Valid = v, true
	return nil
}

var (
	_ json.Unmarshaler      = (*flexNum)(nil)
	_ footysync.MatchSource = (*Provider)(nil)
)

// --- fetching ---

// CompetitionMatches implements provider.MatchesProvider: one request for
// the year, or, when that comes back exactly full — ESPN truncates
// without saying so — twelve month-sized ones. If even a month is full,
// what was fetched is returned with provider.ErrIncomplete, so it can be
// stored without being taken for the whole year.
func (p *Provider) CompetitionMatches(ctx context.Context, ref provider.CompetitionRef, year int) ([]models.Match, []models.Team, error) {
	slug, ok := leagueSlugs[ref.CompetitionID]
	if !ok {
		return nil, nil, fmt.Errorf("espn: competition %q is not covered", ref.CompetitionID)
	}
	comp := competitionFor(ref)
	events, err := p.scoreboard(ctx, slug, strconv.Itoa(year))
	if err != nil {
		return nil, nil, err
	}
	complete := true
	if len(events) >= p.limit {
		events = nil
		seen := map[string]bool{}
		for month := 1; month <= 12; month++ {
			batch, err := p.scoreboard(ctx, slug, fmt.Sprintf("%d%02d", year, month))
			if err != nil {
				return nil, nil, err
			}
			if len(batch) >= p.limit {
				complete = false
			}
			for _, e := range batch {
				if !seen[e.ID] {
					seen[e.ID] = true
					events = append(events, e)
				}
			}
		}
	}
	matches, teams := mapScoreboard(events, comp, p.now().UTC())
	if !complete {
		return matches, teams, fmt.Errorf("espn: %s %d has more matches than ESPN returns at once: %w",
			slug, year, provider.ErrIncomplete)
	}
	return matches, teams, nil
}

func (p *Provider) scoreboard(ctx context.Context, slug, dates string) ([]scoreboardEvent, error) {
	var resp scoreboardResponse
	path := fmt.Sprintf("/site/v2/sports/soccer/%s/scoreboard?dates=%s&limit=%d", slug, dates, p.limit)
	if err := p.client.scoreboard(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("espn: matches %s %s: %w", slug, dates, err)
	}
	return resp.Events, nil
}

// NationalTeams implements provider.NationalTeamsProvider, from the team
// lists in directorySlugs, merged by ESPN ID. Only the first list is
// required; the others fill gaps, and one failing costs only its gaps.
func (p *Provider) NationalTeams(ctx context.Context, gender string) ([]models.Team, error) {
	slugs, ok := directorySlugs[gender]
	if !ok {
		return nil, fmt.Errorf("espn: no national teams for gender %q", gender)
	}
	var out []models.Team
	seen := map[string]bool{}
	for i, slug := range slugs {
		var tl teamsResponse
		if err := p.client.site(ctx, fmt.Sprintf("/site/v2/sports/soccer/%s/teams?limit=500", slug), &tl); err != nil {
			if i == 0 || errors.Is(err, provider.ErrRateLimited) {
				return nil, fmt.Errorf("espn: national teams (%s): %w", gender, err)
			}
			continue
		}
		for _, s := range tl.Sports {
			for _, l := range s.Leagues {
				for _, t := range l.Teams {
					if !seen[t.Team.ID] {
						seen[t.Team.ID] = true
						out = append(out, nationalTeam(t.Team.ID, t.Team.DisplayName, t.Team.ShortDisplayName, gender))
					}
				}
			}
		}
	}
	return out, nil
}

// --- mapping ---

// nationalTeam builds a national team under ESPN's ID. Gender is carried
// by the team (and its canonical ID), so a women's side's name is its
// country's, not "England Women".
func nationalTeam(espnID, name, short, gender string) models.Team {
	trim := func(s string) string { return strings.TrimSpace(strings.TrimSuffix(s, " Women")) }
	return models.Team{ID: teamID(espnID), Name: trim(name), ShortName: trim(short), Gender: gender, National: true}
}

// mapScoreboard turns a year's events into matches, and the teams that
// played them. Each match's season is the edition ESPN files it under,
// which for a tournament held over New Year is the year it began.
func mapScoreboard(events []scoreboardEvent, comp models.Competition, updated time.Time) ([]models.Match, []models.Team) {
	var matches []models.Match
	var teams []models.Team
	seenTeam := map[string]bool{}
	for _, e := range events {
		if len(e.Competitions) == 0 {
			continue
		}
		c := e.Competitions[0]
		var home, away *scoreboardCompetitor
		for i := range c.Competitors {
			switch c.Competitors[i].HomeAway {
			case "home":
				home = &c.Competitors[i]
			case "away":
				away = &c.Competitors[i]
			}
		}
		if home == nil || away == nil || home.Team.ID == "" || away.Team.ID == "" {
			continue
		}
		kickoff, ok := parseKickoff(e.Date, c.Date)
		if !ok {
			continue
		}
		edition := e.Season.Year
		if edition == 0 {
			edition = kickoff.Year()
		}
		m := models.Match{
			ID:            matchID(e.ID),
			CompetitionID: comp.ID,
			SeasonID:      comp.ID + ":" + strconv.Itoa(edition),
			Kickoff:       kickoff,
			Status:        matchStatus(c.Status.Type.Name, c.Status.Type.State, c.Status.Type.Completed),
			HomeTeamID:    teamID(home.Team.ID),
			AwayTeamID:    teamID(away.Team.ID),
			Source:        ProviderName,
			UpdatedAt:     updated,
		}
		groupName := ""
		if c.Group != nil {
			groupName = c.Group.Name
		}
		m.Round = roundLabel(e.Season.Slug, groupName)
		if c.NeutralSite != nil {
			m.Neutral = *c.NeutralSite
		} else {
			m.Neutral = neutralGround(c.Venue.Address.Country, home, away)
		}
		if started(m.Status) {
			m.HomeScore, m.AwayScore = score(home.Score), score(away.Score)
			if m.Status == models.MatchPens && home.ShootoutScore != nil && away.ShootoutScore != nil {
				m.HomePens, m.AwayPens = score(*home.ShootoutScore), score(*away.ShootoutScore)
			}
			m.HomeStats, m.AwayStats = matchStats(home), matchStats(away)
		}
		matches = append(matches, m)

		for _, side := range []*scoreboardCompetitor{home, away} {
			if !seenTeam[side.Team.ID] {
				seenTeam[side.Team.ID] = true
				teams = append(teams, nationalTeam(side.Team.ID, side.Team.DisplayName, side.Team.ShortDisplayName, comp.Gender))
			}
		}
	}
	return matches, teams
}

func matchID(espnID string) string { return idPrefix + espnID }

func parseKickoff(values ...string) (time.Time, bool) {
	for _, v := range values {
		for _, layout := range []string{"2006-01-02T15:04Z", time.RFC3339} {
			if t, err := time.Parse(layout, v); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

// matchStatus maps ESPN's status to footy's. Anything ESPN marks
// completed without a status footy knows is a finished match.
func matchStatus(name, state string, completed bool) string {
	switch name {
	case "STATUS_FULL_TIME", "STATUS_FINAL":
		return models.MatchFullTime
	case "STATUS_FINAL_AET":
		return models.MatchAET
	case "STATUS_FINAL_PEN":
		return models.MatchPens
	case "STATUS_POSTPONED", "STATUS_DELAYED":
		return models.MatchPostponed
	case "STATUS_CANCELED", "STATUS_CANCELLED", "STATUS_ABANDONED", "STATUS_FORFEIT":
		return models.MatchCancelled
	}
	switch {
	case state == "in":
		return models.MatchInPlay
	case completed:
		return models.MatchFullTime
	}
	return models.MatchScheduled
}

func started(status string) bool {
	switch status {
	case models.MatchInPlay, models.MatchFullTime, models.MatchAET, models.MatchPens:
		return true
	}
	return false
}

func score(n flexNum) *int {
	if !n.Valid || n.Value < 0 {
		return nil
	}
	v := int(n.Value)
	return &v
}

// roundNames are the round slugs ESPN uses, as footy shows them.
var roundNames = map[string]string{
	"group-stage":     "Group stage",
	"league-stage":    "League stage",
	"round-of-32":     "Round of 32",
	"round-of-16":     "Round of 16",
	"quarterfinals":   "Quarterfinal",
	"semifinals":      "Semifinal",
	"3rd-place-match": "Third place",
	"third-place":     "Third place",
	"final":           "Final",
}

// roundLabel is the match's round: its group when it has one, otherwise
// its stage. A friendly's "stage" is the year, which says nothing.
func roundLabel(slug, group string) string {
	if g := groupLabel(group); g != "" {
		return g
	}
	if name, ok := roundNames[slug]; ok {
		return name
	}
	if slug == "" || strings.Contains(slug, "friendly") || (slug[0] >= '0' && slug[0] <= '9') {
		return ""
	}
	words := strings.Fields(strings.ReplaceAll(slug, "-", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// groupLabel tidies a group name: ESPN's qualifier groups can read "2026
// FIFA World Cup¤ CAF Qualifying - Group E", which is "Group E".
func groupLabel(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(name, "¤", ""))
	if i := strings.LastIndex(name, " - "); i >= 0 {
		name = strings.TrimSpace(name[i+3:])
	}
	return name
}

// neutralGround reports a match played in a country neither side is
// from. The scoreboard has no neutral flag of its own, but it has the
// venue's country.
func neutralGround(venueCountry string, home, away *scoreboardCompetitor) bool {
	v := textfold.Fold(venueCountry)
	if v == "" {
		return false
	}
	for _, side := range []*scoreboardCompetitor{home, away} {
		if v == textfold.Fold(side.Team.Location) || v == textfold.Fold(side.Team.DisplayName) {
			return false
		}
	}
	return true
}

// matchStats reads one side's numbers. As with club seasons, anything
// impossible is dropped rather than shown: possession outside (0, 100),
// more shots on target than shots, more of anything than a match holds.
// A side whose every figure is zero has no stats, not a match where
// nothing happened.
func matchStats(c *scoreboardCompetitor) *models.MatchTeamStats {
	get := func(name string) (float64, bool) {
		for _, s := range c.Statistics {
			if s.Name == name {
				v, err := strconv.ParseFloat(strings.TrimSuffix(s.DisplayValue, "%"), 64)
				return v, err == nil
			}
		}
		return 0, false
	}
	count := func(name string, max float64) *int {
		v, ok := get(name)
		if !ok || v < 0 || v > max {
			return nil
		}
		n := int(v)
		return &n
	}
	st := &models.MatchTeamStats{
		Shots:         count("totalShots", 60),
		ShotsOnTarget: count("shotsOnTarget", 40),
		Corners:       count("wonCorners", 30),
		Fouls:         count("foulsCommitted", 50),
	}
	if v, ok := get("possessionPct"); ok && v > 0 && v < 100 {
		st.Possession = &v
	}
	if st.Shots != nil && st.ShotsOnTarget != nil && *st.ShotsOnTarget > *st.Shots {
		st.ShotsOnTarget = nil
	}
	zero := func(p *int) bool { return p == nil || *p == 0 }
	if st.Possession == nil && zero(st.Shots) && zero(st.ShotsOnTarget) && zero(st.Corners) && zero(st.Fouls) {
		return nil
	}
	return st
}
