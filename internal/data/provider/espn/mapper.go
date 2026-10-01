package espn

import (
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// leagueSlugs maps canonical competition IDs to ESPN's league slugs. Every
// competition sync.Coverages routes to ESPN needs a row here.
var leagueSlugs = map[string]string{
	"eng-championship":            "eng.2",
	"esp-la-liga-2":               "esp.2",
	"ger-2-bundesliga":            "ger.2",
	"ita-serie-b":                 "ita.2",
	"fra-ligue-2":                 "fra.2",
	"usa-mls":                     "usa.1",
	"usa-usl-championship":        "usa.usl.1",
	"eng-wsl":                     "eng.w.1",
	"esp-liga-f":                  "esp.w.1",
	"fra-premiere-ligue":          "fra.w.1",
	"usa-nwsl":                    "usa.nwsl",
	"eur-champions-league":        "uefa.champions",
	"eur-europa-league":           "uefa.europa",
	"eur-conference-league":       "uefa.europa.conf",
	"eur-womens-champions-league": "uefa.wchampions",
}

// Provider-native IDs. They are not canonical: the sync layer resolves
// them to footy's own team and player IDs (and records the mapping), so
// ESPN's Arsenal in the Champions League is the same team understat's
// Arsenal is in the Premier League.
const (
	ProviderName = "espn"
	idPrefix     = ProviderName + ":"
)

func teamID(espnID string) string   { return idPrefix + espnID }
func playerID(espnID string) string { return idPrefix + espnID }

// --- wire types ---

type stat struct {
	Name  string   `json:"name"`
	Value *float64 `json:"value"`
}

type statList []stat

func (l statList) get(name string) (float64, bool) {
	for _, s := range l {
		if s.Name == name && s.Value != nil {
			return *s.Value, true
		}
	}
	return 0, false
}

type wireTeam struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	ShortDisplayName string `json:"shortDisplayName"`
	Abbreviation     string `json:"abbreviation"`
	Color            string `json:"color"`
	AlternateColor   string `json:"alternateColor"`
}

type standingsResponse struct {
	Children []struct {
		Name      string `json:"name"`
		Standings struct {
			Entries []struct {
				Team  wireTeam `json:"team"`
				Stats statList `json:"stats"`
			} `json:"entries"`
		} `json:"standings"`
	} `json:"children"`
}

type teamsResponse struct {
	Sports []struct {
		Leagues []struct {
			Teams []struct {
				Team wireTeam `json:"team"`
			} `json:"teams"`
		} `json:"leagues"`
	} `json:"sports"`
}

type categories struct {
	Splits struct {
		Categories []struct {
			Stats statList `json:"stats"`
		} `json:"categories"`
	} `json:"splits"`
}

// all flattens every category's stats into one list.
func (c categories) all() statList {
	var out statList
	for _, cat := range c.Splits.Categories {
		out = append(out, cat.Stats...)
	}
	return out
}

type rosterResponse struct {
	Athletes []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		DateOfBirth string `json:"dateOfBirth"`
		Citizenship string `json:"citizenship"`
		Position    struct {
			Abbreviation string `json:"abbreviation"`
		} `json:"position"`
		Statistics categories `json:"statistics"`
	} `json:"athletes"`
}

// --- mapping ---

// mapStandings turns a standings payload into one row per club. A
// competition with groups (MLS's conferences, an old Champions League
// group stage) ranks clubs within their group, and that rank is kept as
// the position: it is what the table on ESPN says.
func mapStandings(resp standingsResponse, comp models.Competition, seasonID string, updated time.Time) ([]models.Team, []models.TeamSeasonStats) {
	var teams []models.Team
	var rows []models.TeamSeasonStats
	for _, group := range resp.Children {
		for _, e := range group.Standings.Entries {
			teams = append(teams, mapTeam(e.Team, comp))
			row := models.TeamSeasonStats{
				TeamID: teamID(e.Team.ID), CompetitionID: comp.ID, SeasonID: seasonID,
				MatchesPlayed: intStat(e.Stats, "gamesPlayed"),
				Wins:          intStat(e.Stats, "wins"),
				Draws:         intStat(e.Stats, "ties"),
				Losses:        intStat(e.Stats, "losses"),
				GoalsFor:      intStat(e.Stats, "pointsFor"),
				GoalsAgainst:  intStat(e.Stats, "pointsAgainst"),
				Points:        intStat(e.Stats, "points"),
				Source:        ProviderName,
				UpdatedAt:     updated,
			}
			row.GoalDiff = row.GoalsFor - row.GoalsAgainst
			if rank, ok := e.Stats.get("rank"); ok && rank > 0 {
				p := int(rank)
				row.Position = &p
			}
			rows = append(rows, row)
		}
	}
	return teams, rows
}

// mapTeam builds a Team. Domestic competitions give a club its country;
// a European competition's clubs come from everywhere, so their country
// is left for identity resolution to supply from the club already cached.
func mapTeam(t wireTeam, comp models.Competition) models.Team {
	team := models.Team{ID: teamID(t.ID), Name: t.DisplayName, ShortName: t.ShortDisplayName, Gender: comp.Gender}
	if comp.Country != "Europe" {
		team.Country = comp.Country
	}
	return team
}

// mapColors extracts brand colors for clubs that have them.
func mapColors(resp teamsResponse) []models.TeamColors {
	var out []models.TeamColors
	for _, s := range resp.Sports {
		for _, l := range s.Leagues {
			for _, t := range l.Teams {
				if !isHex(t.Team.Color) {
					continue
				}
				c := models.TeamColors{TeamID: teamID(t.Team.ID), PrimaryHex: "#" + strings.ToUpper(t.Team.Color), Source: ProviderName}
				if isHex(t.Team.AlternateColor) {
					c.SecondaryHex = "#" + strings.ToUpper(t.Team.AlternateColor)
				}
				out = append(out, c)
			}
		}
	}
	return out
}

func isHex(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// mapRoster turns one club's roster into players and their stat lines.
// ESPN lists the whole registered squad; only players who actually
// appeared are kept, matching what understat supplies.
func mapRoster(resp rosterResponse, comp models.Competition, espnTeamID, seasonID string, updated time.Time) ([]models.Player, []models.PlayerSeasonStats) {
	var players []models.Player
	var lines []models.PlayerSeasonStats
	for _, a := range resp.Athletes {
		st := a.Statistics.all()
		apps := intStat(st, "appearances")
		if apps == 0 {
			continue
		}
		p := models.Player{
			ID: playerID(a.ID), Name: a.DisplayName, Nationality: a.Citizenship,
			Position: position(a.Position.Abbreviation), Gender: comp.Gender,
		}
		if dob, err := time.Parse("2006-01-02T15:04Z", a.DateOfBirth); err == nil {
			d := time.Date(dob.Year(), dob.Month(), dob.Day(), 0, 0, 0, 0, time.UTC)
			p.DateOfBirth = &d
		}
		players = append(players, p)

		line := models.PlayerSeasonStats{
			PlayerID: p.ID, TeamID: teamID(espnTeamID), CompetitionID: comp.ID, SeasonID: seasonID,
			Appearances: apps,
			Goals:       intStat(st, "totalGoals"),
			Assists:     intStat(st, "goalAssists"),
			YellowCards: intStat(st, "yellowCards"),
			RedCards:    intStat(st, "redCards"),
			Source:      ProviderName,
			UpdatedAt:   updated,
		}
		if v, ok := st.get("totalShots"); ok {
			n := int(v)
			line.Shots = &n
		}
		lines = append(lines, line)
	}
	return players, lines
}

// position maps ESPN's one-letter positions to footy's.
func position(abbr string) string {
	switch abbr {
	case "G":
		return "GK"
	case "D", "M", "F":
		return abbr
	}
	return abbr
}

// applyTeamStats fills the columns understat never had — possession,
// corners, fouls, cards, shots — from a club's season statistics.
//
// ESPN's figures here are uneven: a season can report 0% possession, or
// thousands of corners. Every value is checked against what is possible
// for the number of matches played, and anything implausible is left
// unset rather than shown, since a missing number is honest and a wrong
// one isn't.
func applyTeamStats(row *models.TeamSeasonStats, c categories) {
	st := c.all()
	mp := row.MatchesPlayed
	if mp <= 0 {
		return
	}
	if v, ok := st.get("possessionPct"); ok && v >= 20 && v <= 80 {
		row.PossessionAvg = &v
	}
	perGame := func(name string, max float64) *int {
		v, ok := st.get(name)
		if !ok || v <= 0 || v/float64(mp) > max {
			return nil
		}
		n := int(v)
		return &n
	}
	row.Corners = perGame("wonCorners", 15)
	row.FoulsCommitted = perGame("foulsCommitted", 30)
	row.FoulsSuffered = perGame("foulsSuffered", 30)
	row.Shots = perGame("totalShots", 40)
	row.ShotsOnTarget = perGame("shotsOnTarget", 20)
	if row.Shots != nil && row.ShotsOnTarget != nil && *row.ShotsOnTarget > *row.Shots {
		row.ShotsOnTarget = nil
	}
	row.YellowCards = perGame("yellowCards", 8)
	// A season with no red cards is normal, so zero is kept here.
	if v, ok := st.get("redCards"); ok && v >= 0 && v/float64(mp) <= 2 {
		n := int(v)
		row.RedCards = &n
	}
}

func intStat(l statList, name string) int {
	v, _ := l.get(name)
	return int(v)
}
