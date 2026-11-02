package models

import (
	"strconv"
	"time"
)

// MatchDetail is what is known about one match beyond its score and box
// score: the events, every team stat, the lineups and the commentary. It
// is fetched for a club's last game only.
type MatchDetail struct {
	MatchID string
	// Final is set when the detail was fetched after full time; a detail
	// that isn't is fetched again.
	Final      bool
	Venue      string
	Attendance int // 0 when unknown
	Home, Away MatchSide
	// Events are in match order.
	Events []MatchEvent
	// Commentary is in match order.
	Commentary []CommentaryLine
	FetchedAt  time.Time
}

// Empty reports whether there's nothing to show: the source didn't have
// the game, and the empty detail only records that it was asked.
func (d MatchDetail) Empty() bool {
	return len(d.Events) == 0 && len(d.Home.Lineup) == 0 && len(d.Away.Lineup) == 0 && len(d.Commentary) == 0
}

// MatchSide is one team's half of a MatchDetail.
type MatchSide struct {
	Formation string // "4-2-3-1", or "" when unknown
	Stats     MatchDetailStats
	// Lineup is the starters by formation place, then the substitutes.
	Lineup []LineupPlayer
}

// Side returns the home or away half.
func (d MatchDetail) Side(home bool) MatchSide {
	if home {
		return d.Home
	}
	return d.Away
}

// MatchDetailStats is one side's full stat sheet. Each is nil when the
// source had none.
type MatchDetailStats struct {
	Possession        *float64 // percent
	Shots             *int
	ShotsOnTarget     *int
	BlockedShots      *int
	Passes            *int
	AccuratePasses    *int
	Crosses           *int
	AccurateCrosses   *int
	LongBalls         *int
	AccurateLongBalls *int
	Tackles           *int
	TacklesWon        *int
	Interceptions     *int
	Clearances        *int
	Saves             *int
	Offsides          *int
	Corners           *int
	Fouls             *int
	YellowCards       *int
	RedCards          *int
}

// Match event kinds.
const (
	EventGoal         = "goal"
	EventOwnGoal      = "own_goal"
	EventPenaltyGoal  = "penalty_goal"
	EventPenaltyMiss  = "penalty_miss"
	EventYellow       = "yellow"
	EventSecondYellow = "second_yellow"
	EventRed          = "red"
	EventSub          = "sub"
	EventHalfTime     = "half_time"
	EventFullTime     = "full_time"
)

// Sides of a match event; SideNone is a whistle that belongs to neither.
const (
	SideHome = "home"
	SideAway = "away"
	SideNone = ""
)

// MatchEvent is one moment of a match.
type MatchEvent struct {
	Kind string // Event* above
	// Side is the team the event counts for. An own goal counts for the
	// side it was scored for, so Player is on the other side.
	Side string
	// Period is 1 or 2, 3 and 4 for extra time, 5 for a shoot-out.
	Period int
	// Minute is the match minute, stoppage time not included; Added is the
	// stoppage minute ("45'+2'" is 45 and 2).
	Minute, Added int
	// Player did it: the scorer, the player booked, the player coming on.
	Player   string
	PlayerID string // canonical, or "" when not known
	// Other is the assist on a goal, or the player going off on a sub.
	Other   string
	OtherID string
	// Text is the source's description, for the commentary-style row.
	Text string
}

// Clock is the minute as a match report writes it: "16'", "45+2'".
func (e MatchEvent) Clock() string {
	return MinuteLabel(e.Minute, e.Added)
}

// MinuteLabel writes a match minute: "16'", "90+3'".
func MinuteLabel(minute, added int) string {
	if added > 0 {
		return strconv.Itoa(minute) + "+" + strconv.Itoa(added) + "'"
	}
	return strconv.Itoa(minute) + "'"
}

// IsGoal reports whether the event changed the score.
func (e MatchEvent) IsGoal() bool {
	switch e.Kind {
	case EventGoal, EventOwnGoal, EventPenaltyGoal:
		return true
	}
	return false
}

// IsCard reports whether the event is a booking or a sending off.
func (e MatchEvent) IsCard() bool {
	switch e.Kind {
	case EventYellow, EventSecondYellow, EventRed:
		return true
	}
	return false
}

// LineupPlayer is one player in a side's matchday squad.
type LineupPlayer struct {
	Name     string
	PlayerID string // canonical, or "" when not known
	Shirt    string
	Position string // the source's short position: "G", "CD-L", "AM"
	// Place is the formation place, 1 the keeper; 0 for a substitute.
	Place   int
	Starter bool
	// On and Off are the minutes subbed on and off, 0 when not.
	On, Off  int
	Goals    int
	Assists  int
	OwnGoals int
	Yellow   bool
	Red      bool
	Saves    int
}

// Played reports whether the player got on the pitch.
func (p LineupPlayer) Played() bool {
	return p.Starter || p.On > 0
}

// CommentaryLine is one line of minute-by-minute commentary.
type CommentaryLine struct {
	// Clock is the source's minute ("45'+2'"), "" before kick-off.
	Clock string
	// Kind is the play it describes ("Goal", "Foul", "Corner"), "" for
	// narration.
	Kind string
	Text string
}
