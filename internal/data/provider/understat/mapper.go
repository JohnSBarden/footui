package understat

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// leagueKeys maps our canonical competition IDs to understat.com's own
// league key (the path segment in /league/{key} and /getLeagueData/{key}/).
// This is the entire "which leagues does this provider cover" surface —
// extending coverage to another understat.com league is adding one entry
// here, nothing else in the package changes.
var leagueKeys = map[string]string{
	"eng-premier-league": "EPL",
	"esp-la-liga":        "La liga",
	"ger-bundesliga":     "Bundesliga",
	"ita-serie-a":        "Serie A",
	"fra-ligue-1":        "Ligue 1",
	"rus-premier-league": "RFPL",
}

// countryPrefix maps a covered competition to the country-code prefix used
// when generating canonical team IDs (e.g. "eng-manchester-city").
var countryPrefix = map[string]string{
	"eng-premier-league": "eng",
	"esp-la-liga":        "esp",
	"ger-bundesliga":     "ger",
	"ita-serie-a":        "ita",
	"fra-ligue-1":        "fra",
	"rus-premier-league": "rus",
}

// countryName maps a covered competition to its country, for Team.Country.
var countryName = map[string]string{
	"eng-premier-league": "England",
	"esp-la-liga":        "Spain",
	"ger-bundesliga":     "Germany",
	"ita-serie-a":        "Italy",
	"fra-ligue-1":        "France",
	"rus-premier-league": "Russia",
}

// leagueData is the raw shape of a GET /getLeagueData/{league}/{season}
// response. Only the fields footui uses are modeled; unrecognized fields
// (understat.com also returns "dates", used for its calendar view) are
// ignored by encoding/json rather than erroring.
type leagueData struct {
	Teams   map[string]teamEntry `json:"teams"`
	Players []playerEntry        `json:"players"`
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

// ppdaEntry is understat.com's per-match pressing figure: Att is the
// opponent's passes in their own build-up area, Def is our defensive
// actions there. The published PPDA statistic is Att/Def.
type ppdaEntry struct {
	Att int `json:"att"`
	Def int `json:"def"`
}

// playerEntry's numeric fields arrive as JSON strings from understat.com,
// so they're decoded as strings and parsed explicitly in the mapper.
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

// optionalInt parses one of understat.com's string-encoded numbers,
// returning nil when the field is absent or unparseable. Nil is the right
// answer rather than zero: "this provider didn't tell us" and "the player
// took zero shots" are different facts, and screens render them
// differently.
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

// primaryTeamTitle picks one club from understat.com's team_title field,
// which is comma-joined ("Bournemouth,Manchester City") for a player who
// changed clubs mid-season. It returns the last-listed (most recent) club.
// The season stat line understat.com gives for that player is still a
// single aggregate across every club they played for — attributing it
// entirely to the most recent club is an approximation, not a split by
// club, and is called out in docs/data-model.md.
func primaryTeamTitle(teamTitle string) string {
	parts := strings.Split(teamTitle, ",")
	return strings.TrimSpace(parts[len(parts)-1])
}

// teamID generates understat's own ID for a team: "understat:" and the
// canonical ID footy has always derived from the club's name within its
// competition's country namespace. The sync layer resolves it like any
// provider's ID, so a club another provider cached first is matched
// rather than duplicated, and a club footy hasn't seen keeps exactly the
// canonical ID it always had.
func teamID(compID, teamTitle string) string {
	prefix := countryPrefix[compID]
	if prefix == "" {
		prefix = "team"
	}
	return "understat:" + prefix + "-" + slugify(teamTitle)
}

// mapTeamEntities extracts the Team rows (identity, not stats) referenced
// by a league/season payload.
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

// mapStandings aggregates each team's per-match history into one
// TeamSeasonStats row per team, then assigns Position by sorting on points,
// goal difference, and goals for — understat.com doesn't return a rank
// directly, so this is footui's own computed ranking (ties beyond these
// three keys, e.g. head-to-head, aren't modeled).
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
			// PPDA is a ratio, so a season figure is the ratio of the
			// summed numerators and denominators — averaging the
			// per-match ratios would weight a quiet match as heavily as a
			// frantic one and give a different, wrong answer.
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

// mapPlayerEntities extracts the Player rows (identity/bio) referenced by a
// league/season's roster, separate from their season stat lines.
// understat.com only gives a position string and no bio fields beyond name.
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
