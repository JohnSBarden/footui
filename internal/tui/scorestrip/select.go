// Package scorestrip is the row of score tiles across the top of the
// matchday and league screens.
package scorestrip

import (
	"slices"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// roundSpan covers a weekend round, Friday night to Monday night.
const roundSpan = 84 * time.Hour

// liveFor is how long after kickoff a game without a result counts as
// still going on.
const liveFor = 3 * time.Hour

// Games either side of now: team* on the matchday strip, page* on a
// team's own page.
const (
	teamRecent   = 2
	teamUpcoming = 1
	pageRecent   = 10
	pageUpcoming = 5
)

// Rounds is a season's latest round played by now and its next one.
func Rounds(matches []models.Match, now time.Time) []models.Match {
	var last, next time.Time
	for _, m := range matches {
		switch {
		case !m.Kickoff.After(now):
			if m.Kickoff.After(last) {
				last = m.Kickoff
			}
		case next.IsZero() || m.Kickoff.Before(next):
			next = m.Kickoff
		}
	}
	var out []models.Match
	for _, m := range matches {
		played := !m.Kickoff.After(now)
		if played && !last.IsZero() && m.Kickoff.After(last.Add(-roundSpan)) ||
			!played && !next.IsZero() && !m.Kickoff.After(next.Add(roundSpan)) {
			out = append(out, m)
		}
	}
	return Ordered(out)
}

// TeamWindow is a team's last few games and its next, for the matchday
// strip.
func TeamWindow(matches []models.Match, now time.Time) []models.Match {
	return window(matches, now, teamRecent, teamUpcoming)
}

// Schedule is the strip on a team's own page.
func Schedule(matches []models.Match, now time.Time) []models.Match {
	return window(matches, now, pageRecent, pageUpcoming)
}

func window(matches []models.Match, now time.Time, recent, upcoming int) []models.Match {
	ms := Ordered(matches)
	split, _ := slices.BinarySearchFunc(ms, now, func(m models.Match, t time.Time) int {
		if m.Kickoff.After(t) {
			return 1
		}
		return -1
	})
	return slices.Clone(ms[max(split-recent, 0):min(split+upcoming, len(ms))])
}

// Ordered is matches without repeats, in kickoff order.
func Ordered(matches []models.Match) []models.Match {
	seen := make(map[string]bool, len(matches))
	out := make([]models.Match, 0, len(matches))
	for _, m := range matches {
		if !seen[m.ID] {
			seen[m.ID] = true
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(a, b models.Match) int {
		if c := a.Kickoff.Compare(b.Kickoff); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// current reports whether a game is in play or still to come.
func current(m models.Match, now time.Time) bool {
	switch {
	case m.Finished(), m.Status == models.MatchPostponed, m.Status == models.MatchCancelled:
		return false
	case m.Status == models.MatchInPlay:
		return true
	}
	return m.Kickoff.After(now.Add(-liveFor))
}
