package league

import "strconv"

// zone is what a final position leads to, where footui knows the rule.
type zone int

const (
	zoneNone zone = iota
	zoneUCL
	zoneUEL
	zoneDown
)

// zoneRule is one competition's places from a season on. Places won
// through cups or coefficients aren't knowable from the table, and
// play-off places depend on a later tie, so only the places a league
// position decides outright are seeded, and only from the season the
// current format began. An unknown competition or an earlier season shows
// no zones: a guessed zone would be a wrong one.
type zoneRule struct {
	from         int // first season start year the rule holds for
	ucl, uel     int // positions 1..ucl, then ucl+1..uel
	relegateFrom int // this position and below go down
}

var zoneRules = map[string]zoneRule{
	"eng-premier-league": {from: 2024, ucl: 4, uel: 5, relegateFrom: 18},
	"esp-la-liga":        {from: 2024, ucl: 4, uel: 5, relegateFrom: 18},
	"ita-serie-a":        {from: 2024, ucl: 4, uel: 5, relegateFrom: 18},
	"ger-bundesliga":     {from: 2024, ucl: 4, uel: 5, relegateFrom: 17},
	"fra-ligue-1":        {from: 2024, ucl: 4, uel: 5, relegateFrom: 17},
}

// hasZones reports whether a competition-season has seeded zones.
func hasZones(competitionID string, startYear int) bool {
	r, ok := zoneRules[competitionID]
	return ok && startYear >= r.from
}

func zoneFor(competitionID string, startYear, position int) zone {
	if !hasZones(competitionID, startYear) || position <= 0 {
		return zoneNone
	}
	r := zoneRules[competitionID]
	switch {
	case position <= r.ucl:
		return zoneUCL
	case position <= r.uel:
		return zoneUEL
	case position >= r.relegateFrom:
		return zoneDown
	}
	return zoneNone
}

// startYear is the year a season label begins in: 2024 for "2024-25" and
// for a calendar-year "2024"; 0 when the label doesn't start with one.
func startYear(label string) int {
	if len(label) < 4 {
		return 0
	}
	y, err := strconv.Atoi(label[:4])
	if err != nil {
		return 0
	}
	return y
}
