package espn

import (
	"sort"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// statSlugs are understat's leagues, whose player season detail still
// comes from ESPN. Kept out of leagueSlugs, which routes ESPN syncs.
var statSlugs = map[string]string{
	"eng-premier-league": "eng.1",
	"esp-la-liga":        "esp.1",
	"ger-bundesliga":     "ger.1",
	"ita-serie-a":        "ita.1",
	"fra-ligue-1":        "fra.1",
	"rus-premier-league": "rus.1",
}

func slugFor(competitionID string) (string, bool) {
	if s, ok := statSlugs[competitionID]; ok {
		return s, true
	}
	s, ok := leagueSlugs[competitionID]
	return s, ok
}

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

	// Domestic cups, a club's matches only.
	"eng-fa-cup":          "eng.fa",
	"eng-league-cup":      "eng.league_cup",
	"esp-copa-del-rey":    "esp.copa_del_rey",
	"ger-dfb-pokal":       "ger.dfb_pokal",
	"ita-coppa-italia":    "ita.coppa_italia",
	"fra-coupe-de-france": "fra.coupe_de_france",
	"usa-open-cup":        "usa.open",

	// International. "uefa.w.nations" and "afc.asian.cup" are right; the
	// obvious spellings return 400.
	"world-cup":               "fifa.world",
	"euro":                    "uefa.euro",
	"copa-america":            "conmebol.america",
	"gold-cup":                "concacaf.gold",
	"afcon":                   "caf.nations",
	"asian-cup":               "afc.asian.cup",
	"wcq-uefa":                "fifa.worldq.uefa",
	"wcq-conmebol":            "fifa.worldq.conmebol",
	"wcq-concacaf":            "fifa.worldq.concacaf",
	"wcq-afc":                 "fifa.worldq.afc",
	"wcq-caf":                 "fifa.worldq.caf",
	"euro-qualifying":         "uefa.euroq",
	"nations-league":          "uefa.nations",
	"concacaf-nations-league": "concacaf.nations.league",
	"int-friendly":            "fifa.friendly",
	"world-cup-women":         "fifa.wwc",
	"euro-women":              "uefa.weuro",
	"nations-league-women":    "uefa.w.nations",
	"int-friendly-women":      "fifa.friendly.w",
}

// directorySlugs list each gender's national teams. The women's
// friendlies miss some nations (England), so two more are merged in.
var directorySlugs = map[string][]string{
	"men":   {"fifa.friendly"},
	"women": {"fifa.friendly.w", "fifa.wwc", "uefa.w.nations"},
}

// Provider-native IDs; the sync layer resolves them to canonical ones.
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

// mapStandings turns a standings payload into one row per club.
func mapStandings(resp standingsResponse, comp models.Competition, seasonID string, updated time.Time) ([]models.Team, []models.TeamSeasonStats) {
	var teams []models.Team
	var rows []models.TeamSeasonStats
	// A tournament's groups keep their own ranking; a club competition's
	// groups (MLS conferences) keep labels but are ranked as one table.
	groups := len(resp.Children) > 1
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
			if groups {
				row.GroupLabel = groupLabel(group.Name)
			}
			rows = append(rows, row)
		}
	}
	if groups && !comp.International() {
		rankOverall(rows)
	}
	return teams, rows
}

// rankOverall replaces per-group ranks with one table across all groups,
// so two conferences don't interleave as "1, 1, 2, 2".
func rankOverall(rows []models.TeamSeasonStats) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		if a.GoalDiff != b.GoalDiff {
			return a.GoalDiff > b.GoalDiff
		}
		return a.GoalsFor > b.GoalsFor
	})
	for i := range rows {
		p := i + 1
		rows[i].Position = &p
	}
}

// mapTeam builds a Team. A European competition's clubs get no country;
// identity resolution supplies it.
func mapTeam(t wireTeam, comp models.Competition) models.Team {
	if comp.International() {
		return nationalTeam(t.ID, t.DisplayName, t.ShortDisplayName, comp.Gender)
	}
	team := models.Team{ID: teamID(t.ID), Name: t.DisplayName, ShortName: t.ShortDisplayName, Gender: comp.Gender}
	if comp.Country != "Europe" {
		team.Country = comp.Country
	}
	return team
}

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

// mapRoster turns one club's roster into players and stat lines. ESPN
// lists the whole squad; only players who appeared are kept.
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

// position maps ESPN's one-letter positions to footui's.
func position(abbr string) string {
	switch abbr {
	case "G":
		return "GK"
	case "D", "M", "F":
		return abbr
	}
	return abbr
}

// applyTeamStats fills possession, corners, fouls, cards and shots from a
// club's season statistics. ESPN's figures can be absurd (0% possession,
// thousands of corners), so implausible values are left unset.
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
