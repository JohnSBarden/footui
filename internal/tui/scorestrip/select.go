// Package scorestrip is the row of scores across the top of the matchday
// and league screens: the latest results, any game in play and the next
// fixtures, a tile each, the way a league's own website runs them along
// its header.
package scorestrip

import (
	"slices"
	"strings"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// roundSpan is how far either side of a round's first or last kickoff its
// other games can fall: a weekend round runs Friday night to Monday night.
const roundSpan = 84 * time.Hour

// liveFor is how long after kickoff a game without a result is taken to
// still be going on, so the strip opens with it in view.
const liveFor = 3 * time.Hour

// Per team, the strip carries its last few results and its next fixture.
const (
	teamRecent   = 2
	teamUpcoming = 1
)

// Rounds is a season's latest round played by now and its next one: the
// games within roundSpan of the last kickoff before now, and of the first
// after it. A season that has ended keeps only its last round, one yet to
// start only its first.
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

// TeamWindow is a team's last teamRecent games kicked off by now and its
// next teamUpcoming, from its matches in any order.
func TeamWindow(matches []models.Match, now time.Time) []models.Match {
	ms := Ordered(matches)
	split, _ := slices.BinarySearchFunc(ms, now, func(m models.Match, t time.Time) int {
		if m.Kickoff.After(t) {
			return 1
		}
		return -1
	})
	return slices.Clone(ms[max(split-teamRecent, 0):min(split+teamUpcoming, len(ms))])
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

// current reports whether a game is in play or still to come: what the
// strip opens with in view.
func current(m models.Match, now time.Time) bool {
	switch {
	case m.Finished(), m.Status == models.MatchPostponed, m.Status == models.MatchCancelled:
		return false
	case m.Status == models.MatchInPlay:
		return true
	}
	return m.Kickoff.After(now.Add(-liveFor))
}
