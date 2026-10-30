package understat

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// leagueKeys maps canonical competition IDs to understat's league keys.
// It defines this provider's coverage.
var leagueKeys = map[string]string{
	"eng-premier-league": "EPL",
	"esp-la-liga":        "La liga",
	"ger-bundesliga":     "Bundesliga",
	"ita-serie-a":        "Serie A",
	"fra-ligue-1":        "Ligue 1",
	"rus-premier-league": "RFPL",
}

// countryPrefix is the team ID prefix ("eng-manchester-city").
var countryPrefix = map[string]string{
	"eng-premier-league": "eng",
	"esp-la-liga":        "esp",
	"ger-bundesliga":     "ger",
	"ita-serie-a":        "ita",
	"fra-ligue-1":        "fra",
	"rus-premier-league": "rus",
}

var countryName = map[string]string{
	"eng-premier-league": "England",
	"esp-la-liga":        "Spain",
	"ger-bundesliga":     "Germany",
	"ita-serie-a":        "Italy",
	"fra-ligue-1":        "France",
	"rus-premier-league": "Russia",
}

// leagueData is a /getLeagueData/{league}/{season} response.
type leagueData struct {
	Teams   map[string]teamEntry `json:"teams"`
	Players []playerEntry        `json:"players"`
	// Dates is every match of the season, played or not.
	Dates []dateEntry `json:"dates"`
}

// dateEntry is one match. Goals and xG arrive as strings, null until the
// match is played; datetime is UTC.
type dateEntry struct {
	ID       string      `json:"id"`
	IsResult bool        `json:"isResult"`
	Home     dateTeam    `json:"h"`
	Away     dateTeam    `json:"a"`
	Goals    dateFigures `json:"goals"`
	XG       dateFigures `json:"xG"`
	Datetime string      `json:"datetime"`
}

type dateTeam struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type dateFigures struct {
	Home *string `json:"h"`
	Away *string `json:"a"`
}

type teamEntry struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	History []matchEntry `json:"history"`
}

type matchEntry struct {
	XG          float64   `json:"xG"`
	XGA         float64   `json:"xGA"`
	NPxG        float64   `json:"npxG"`
	NPxGA       float64   `json:"npxGA"`
	PPDA        ppdaEntry `json:"ppda"`
	PPDAAllowed ppdaEntry `json:"ppda_allowed"`
	Deep        int       `json:"deep"`
	DeepAllowed int       `json:"deep_allowed"`
	XPts        float64   `json:"xpts"`
	Scored      int       `json:"scored"`
	Missed      int       `json:"missed"`
	Wins        int       `json:"wins"`
	Draws       int       `json:"draws"`
	Loses       int       `json:"loses"`
	Pts         int       `json:"pts"`
	Date        string    `json:"date"`
}

// ppdaEntry is a per-match pressing figure; PPDA is Att/Def.
type ppdaEntry struct {
	Att int `json:"att"`
	Def int `json:"def"`
}

// playerEntry's numeric fields arrive as JSON strings.
type playerEntry struct {
	ID          string `json:"id"`
	PlayerName  string `json:"player_name"`
	Games       string `json:"games"`
	Time        string `json:"time"`
	Goals       string `json:"goals"`
	XG          string `json:"xG"`
	Assists     string `json:"assists"`
	XA          string `json:"xA"`
	Shots       string `json:"shots"`
	KeyPasses   string `json:"key_passes"`
	NPG         string `json:"npg"`
	NPxG        string `json:"npxG"`
	YellowCards string `json:"yellow_cards"`
	RedCards    string `json:"red_cards"`
	Position    string `json:"position"`
	TeamTitle   string `json:"team_title"`
}

// optionalInt parses a string-encoded number; nil (not zero) when absent
// or unparseable.
func optionalInt(raw string) *int {
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &v
}

func optionalFloat(raw string) *float64 {
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugNonAlnum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// primaryTeamTitle returns the last-listed club of a comma-joined
// team_title (understat's order isn't always chronological): where a mid-season mover's aggregate line goes when it
// can't be split by club (see splitMovers).
func primaryTeamTitle(teamTitle string) string {
	parts := strings.Split(teamTitle, ",")
	return strings.TrimSpace(parts[len(parts)-1])
}

// teamID is "understat:" plus the canonical ID derived from the club's
// name; changing it would change existing clubs' canonical IDs.
func teamID(compID, teamTitle string) string {
	prefix := countryPrefix[compID]
	if prefix == "" {
		prefix = "team"
	}
	return "understat:" + prefix + "-" + slugify(teamTitle)
}

func mapTeamEntities(data *leagueData, compID string) []models.Team {
	country := countryName[compID]
	out := make([]models.Team, 0, len(data.Teams))
	for _, t := range data.Teams {
		out = append(out, models.Team{
			ID:      teamID(compID, t.Title),
			Name:    t.Title,
			Country: country,
			Gender:  "men",
		})
	}
	return out
}

// mapStandings aggregates each team's match history. understat has no
// rank, so Position is computed by points, goal difference, goals for.
func mapStandings(data *leagueData, compID, seasonID string) []models.TeamSeasonStats {
	now := time.Now().UTC()
	stats := make([]models.TeamSeasonStats, 0, len(data.Teams))

	for _, t := range data.Teams {
		s := models.TeamSeasonStats{
			TeamID:        teamID(compID, t.Title),
			CompetitionID: compID,
			SeasonID:      seasonID,
			MatchesPlayed: len(t.History),
			Source:        "understat",
			UpdatedAt:     now,
		}

		var xg, xga, npxg, npxga, xpts float64
		var deep, deepAllowed int
		var ppdaAtt, ppdaDef, allowedAtt, allowedDef int

		for _, m := range t.History {
			s.Wins += m.Wins
			s.Draws += m.Draws
			s.Losses += m.Loses
			s.GoalsFor += m.Scored
			s.GoalsAgainst += m.Missed
			s.Points += m.Pts
			xg += m.XG
			xga += m.XGA
			npxg += m.NPxG
			npxga += m.NPxGA
			xpts += m.XPts
			deep += m.Deep
			deepAllowed += m.DeepAllowed
			// Season PPDA is the ratio of sums, not the mean of ratios.
			ppdaAtt += m.PPDA.Att
			ppdaDef += m.PPDA.Def
			allowedAtt += m.PPDAAllowed.Att
			allowedDef += m.PPDAAllowed.Def
		}

		s.GoalDiff = s.GoalsFor - s.GoalsAgainst
		s.XG = &xg
		s.XGA = &xga

		if len(t.History) > 0 {
			extras := map[string]float64{
				models.StatNPxG:           npxg,
				models.StatNPxGA:          npxga,
				models.StatExpectedPoints: xpts,
				models.StatDeep:           float64(deep),
				models.StatDeepAllowed:    float64(deepAllowed),
			}
			if ppdaDef > 0 {
				extras[models.StatPPDA] = float64(ppdaAtt) / float64(ppdaDef)
			}
			if allowedDef > 0 {
				extras[models.StatPPDAAllowed] = float64(allowedAtt) / float64(allowedDef)
			}
			s.Extras = extras
		}

		stats = append(stats, s)
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Points != stats[j].Points {
			return stats[i].Points > stats[j].Points
		}
		if stats[i].GoalDiff != stats[j].GoalDiff {
			return stats[i].GoalDiff > stats[j].GoalDiff
		}
		return stats[i].GoalsFor > stats[j].GoalsFor
	})
	for i := range stats {
		pos := i + 1
		stats[i].Position = &pos
	}

	return stats
}

// mapPlayerEntities extracts Player rows; understat has no bio beyond name
// and position.
func mapPlayerEntities(data *leagueData) []models.Player {
	out := make([]models.Player, 0, len(data.Players))
	for _, p := range data.Players {
		out = append(out, models.Player{
			ID:       "understat:" + p.ID,
			Name:     p.PlayerName,
			Position: p.Position,
			Gender:   "men", // all six understat.com-covered leagues are men's competitions
		})
	}
	return out
}

func mapPlayers(data *leagueData, compID, seasonID string) []models.PlayerSeasonStats {
	now := time.Now().UTC()
	out := make([]models.PlayerSeasonStats, 0, len(data.Players))

	for _, p := range data.Players {
		games, _ := strconv.Atoi(p.Games)
		minutes, _ := strconv.Atoi(p.Time)
		goals, _ := strconv.Atoi(p.Goals)
		assists, _ := strconv.Atoi(p.Assists)
		yellow, _ := strconv.Atoi(p.YellowCards)
		red, _ := strconv.Atoi(p.RedCards)
		xg, _ := strconv.ParseFloat(p.XG, 64)
		xa, _ := strconv.ParseFloat(p.XA, 64)

		out = append(out, models.PlayerSeasonStats{
			PlayerID:        "understat:" + p.ID,
			TeamID:          teamID(compID, primaryTeamTitle(p.TeamTitle)),
			CompetitionID:   compID,
			SeasonID:        seasonID,
			Appearances:     games,
			Minutes:         minutes,
			Goals:           goals,
			Assists:         assists,
			XG:              &xg,
			XA:              &xa,
			Shots:           optionalInt(p.Shots),
			KeyPasses:       optionalInt(p.KeyPasses),
			NonPenaltyGoals: optionalInt(p.NPG),
			NPxG:            optionalFloat(p.NPxG),
			YellowCards:     yellow,
			RedCards:        red,
			Source:          "understat",
			UpdatedAt:       now,
		})
	}

	return out
}

// mapMatches turns the season's calendar into matches. A match naming a
// club the payload has no team for is dropped.
func mapMatches(data *leagueData, compID, seasonID string, updated time.Time) []models.Match {
	known := map[string]bool{}
	for _, t := range data.Teams {
		known[t.ID] = true
	}
	out := make([]models.Match, 0, len(data.Dates))
	for _, d := range data.Dates {
		if d.ID == "" || !known[d.Home.ID] || !known[d.Away.ID] {
			continue
		}
		kickoff, err := time.Parse("2006-01-02 15:04:05", d.Datetime)
		if err != nil {
			continue
		}
		m := models.Match{
			ID:            "understat:" + d.ID,
			CompetitionID: compID,
			SeasonID:      seasonID,
			Kickoff:       kickoff.UTC(),
			Status:        models.MatchScheduled,
			HomeTeamID:    teamID(compID, d.Home.Title),
			AwayTeamID:    teamID(compID, d.Away.Title),
			Source:        "understat",
			UpdatedAt:     updated,
		}
		home, away := figure(d.Goals.Home), figure(d.Goals.Away)
		if d.IsResult && home != nil && away != nil {
			m.Status = models.MatchFullTime
			h, a := int(*home), int(*away)
			m.HomeScore, m.AwayScore = &h, &a
			if xg := figure(d.XG.Home); xg != nil {
				m.HomeStats = &models.MatchTeamStats{XG: xg}
			}
			if xg := figure(d.XG.Away); xg != nil {
				m.AwayStats = &models.MatchTeamStats{XG: xg}
			}
		}
		out = append(out, m)
	}
	return out
}

// figure reads one of a match's string-encoded numbers; nil when absent,
// unreadable or negative.
func figure(raw *string) *float64 {
	if raw == nil {
		return nil
	}
	v := optionalFloat(*raw)
	if v == nil || *v < 0 {
		return nil
	}
	return v
}
