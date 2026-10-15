package models

import (
	"slices"
	"time"
)

// Match is one game between two teams. National teams are tracked at this
// grain — every match is stored, and a nation's record is computed from
// whichever of its matches a filter selects — rather than as a season line
// per competition, because a nation's year is a dozen matches spread over
// half a dozen competitions.
type Match struct {
	ID            string
	CompetitionID string
	// SeasonID is the competition's edition the match belongs to, which is
	// not always the calendar year it was played in: Euro 2020 was played
	// in 2021.
	SeasonID string
	Kickoff  time.Time
	Status   string // MatchStatus* below
	// Round is a display label: "Group B", "Quarterfinal", or empty when
	// the source has none (a friendly).
	Round      string
	HomeTeamID string
	AwayTeamID string
	// Neutral is a match at a ground that is neither side's.
	Neutral bool
	// Scores are nil until the match has started. For a match decided on
	// penalties they are the score after extra time, and the shoot-out
	// is in HomePens/AwayPens.
	HomeScore *int
	AwayScore *int
	HomePens  *int
	AwayPens  *int
	// HomeStats and AwayStats are nil when the source had none.
	HomeStats *MatchTeamStats
	AwayStats *MatchTeamStats
	Source    string
	UpdatedAt time.Time
}

// MatchTeamStats is one side's numbers from one match. Each is nil when
// missing or implausible, so averages can skip it rather than count it as
// zero.
type MatchTeamStats struct {
	Possession    *float64
	Shots         *int
	ShotsOnTarget *int
	Corners       *int
	Fouls         *int
	// XG is the side's expected goals, which only understat's league
	// matches carry.
	XG *float64
}

// Match statuses.
const (
	MatchScheduled = "scheduled"
	MatchInPlay    = "in_play"
	MatchFullTime  = "ft"
	// MatchAET is a match won in extra time.
	MatchAET = "aet"
	// MatchPens is a match decided on penalties.
	MatchPens      = "pens"
	MatchPostponed = "postponed"
	MatchCancelled = "cancelled"
)

// Finished reports whether the match has a final result.
func (m Match) Finished() bool {
	switch m.Status {
	case MatchFullTime, MatchAET, MatchPens:
		return m.HomeScore != nil && m.AwayScore != nil
	}
	return false
}

// Involves reports whether teamID played in the match.
func (m Match) Involves(teamID string) bool {
	return m.HomeTeamID == teamID || m.AwayTeamID == teamID
}

// Opponent is the side teamID played against.
func (m Match) Opponent(teamID string) string {
	if m.HomeTeamID == teamID {
		return m.AwayTeamID
	}
	return m.HomeTeamID
}

// For returns teamID's goals, its opponent's, and its stats.
func (m Match) For(teamID string) (scored, conceded *int, stats *MatchTeamStats) {
	if m.HomeTeamID == teamID {
		return m.HomeScore, m.AwayScore, m.HomeStats
	}
	return m.AwayScore, m.HomeScore, m.AwayStats
}

// Result is "W", "D" or "L" for teamID, or "" if the match isn't
// finished. A shoot-out counts as a draw, as FIFA records it: the match
// was level when it ended.
func (m Match) Result(teamID string) string {
	if !m.Finished() {
		return ""
	}
	scored, conceded, _ := m.For(teamID)
	switch {
	case m.Status == MatchPens || *scored == *conceded:
		return "D"
	case *scored > *conceded:
		return "W"
	default:
		return "L"
	}
}

// Filter presets for a nation's matches.
const (
	PresetAll         = "all"
	PresetCompetitive = "competitive"
	PresetTournaments = "tournaments"
)

// MatchFilter selects which of a team's matches count: a preset, or an
// explicit list of competitions. The zero value is every match.
type MatchFilter struct {
	// Preset is one of the Preset* constants; empty with no Competitions
	// is the same as PresetAll.
	Preset string
	// Competitions, when Preset is empty, are the competition IDs to keep.
	Competitions []string
}

// IsAll reports whether the filter keeps every match.
func (f MatchFilter) IsAll() bool {
	return f.Preset == PresetAll || (f.Preset == "" && f.Competitions == nil)
}

// Keeps reports whether a match in comp passes the filter.
func (f MatchFilter) Keeps(comp Competition) bool {
	switch f.Preset {
	case PresetCompetitive:
		return comp.Category == CategoryTournament || comp.Category == CategoryQualifier ||
			comp.Category == CategoryNationsLeague
	case PresetTournaments:
		return comp.Category == CategoryTournament
	case PresetAll:
		return true
	}
	if f.Competitions == nil {
		return true
	}
	return slices.Contains(f.Competitions, comp.ID)
}

// MatchSummary is a team's record over a set of matches.
type MatchSummary struct {
	Played, Won, Drawn, Lost int
	GoalsFor, GoalsAgainst   int
	// Form is the results of the last five finished matches, oldest
	// first, so it reads left to right in time.
	Form string
	// Averages per match, over only the matches that have the stat; nil
	// when none do.
	Possession    *float64
	Shots         *float64
	ShotsOnTarget *float64
	Corners       *float64
	Fouls         *float64
}

// SummarizeMatches computes teamID's record from matches, in any order.
// Matches that haven't finished are ignored.
func SummarizeMatches(teamID string, matches []Match) MatchSummary {
	finished := make([]Match, 0, len(matches))
	for _, m := range matches {
		if m.Involves(teamID) && m.Finished() {
			finished = append(finished, m)
		}
	}
	slices.SortStableFunc(finished, func(a, b Match) int { return a.Kickoff.Compare(b.Kickoff) })

	var s MatchSummary
	var poss, shots, sot, corners, fouls mean
	for _, m := range finished {
		scored, conceded, st := m.For(teamID)
		s.Played++
		s.GoalsFor += *scored
		s.GoalsAgainst += *conceded
		switch m.Result(teamID) {
		case "W":
			s.Won++
		case "D":
			s.Drawn++
		default:
			s.Lost++
		}
		if st != nil {
			poss.addFloat(st.Possession)
			shots.add(st.Shots)
			sot.add(st.ShotsOnTarget)
			corners.add(st.Corners)
			fouls.add(st.Fouls)
		}
	}
	for _, m := range finished[max(len(finished)-5, 0):] {
		s.Form += m.Result(teamID)
	}
	s.Possession, s.Shots, s.ShotsOnTarget = poss.value(), shots.value(), sot.value()
	s.Corners, s.Fouls = corners.value(), fouls.value()
	return s
}

// mean averages the values it is given, skipping missing ones.
type mean struct {
	sum float64
	n   int
}

func (a *mean) add(v *int) {
	if v != nil {
		a.sum += float64(*v)
		a.n++
	}
}

func (a *mean) addFloat(v *float64) {
	if v != nil {
		a.sum += *v
		a.n++
	}
}

func (a mean) value() *float64 {
	if a.n == 0 {
		return nil
	}
	v := a.sum / float64(a.n)
	return &v
}
