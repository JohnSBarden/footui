package components

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// Score renders a match's score as the home side sees it: "2–1",
// "2–1 aet", "1–1 (4–5 p)". An unstarted match renders its state
// ("postp.") or "" for a fixture.
func Score(m models.Match) string {
	return scoreText(m, m.HomeScore, m.AwayScore, m.HomePens, m.AwayPens)
}

// ScoreFor renders the score from teamID's side: its goals first.
func ScoreFor(m models.Match, teamID string) string {
	if m.AwayTeamID == teamID {
		return scoreText(m, m.AwayScore, m.HomeScore, m.AwayPens, m.HomePens)
	}
	return Score(m)
}

func scoreText(m models.Match, a, b, ap, bp *int) string {
	switch m.Status {
	case models.MatchPostponed:
		return "postp."
	case models.MatchCancelled:
		return "canc."
	}
	if a == nil || b == nil {
		return ""
	}
	s := strconv.Itoa(*a) + "–" + strconv.Itoa(*b)
	switch m.Status {
	case models.MatchAET:
		s += " aet"
	case models.MatchPens:
		if ap != nil && bp != nil {
			s += fmt.Sprintf(" (%d–%d p)", *ap, *bp)
		} else {
			s += " pens"
		}
	case models.MatchInPlay:
		s += " live"
	}
	return s
}

// GroupStage is the stage every group match is listed under.
const GroupStage = "Group stage"

// Stage is GroupStage for any group match, else its round, else "Matches".
func Stage(m models.Match) string {
	switch {
	case strings.HasPrefix(m.Round, "Group ") || m.Round == GroupStage:
		return GroupStage
	case m.Round == "":
		return "Matches"
	}
	return m.Round
}

// Kickoff renders a kickoff in local time: "Sat 14 Nov 19:45".
func Kickoff(t time.Time) string {
	return t.Local().Format("Mon 2 Jan 15:04")
}

// MatchDate is the day a match was played, in the viewer's time zone.
func MatchDate(t time.Time) string {
	return t.Local().Format("2006-01-02")
}
