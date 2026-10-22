// Package ticker is the news line beneath the footer for followed teams.
// News is a diff of snapshots taken before and after a refresh batch, so a
// result is news once, when it lands.
package ticker

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
	"github.com/JohnSBarden/footui/internal/tui/components"
)

// recent is how old a result can be and still be news: a whole-history
// refresh fills in years of results, and none of those are.
const recent = 4 * 24 * time.Hour

// Snapshot holds the followed teams' recent results and standings.
type Snapshot struct {
	taken    bool
	finished map[string]models.Match // by match ID
	// standing is each team's newest season per league, keyed
	// team|competition.
	standing map[string]standing
}

type standing struct {
	teamID, compID, seasonID string
	pos                      int
}

// Take snapshots the teams the user follows.
func Take(ctx context.Context, repos storage.Repositories, now time.Time) Snapshot {
	s := Snapshot{taken: true, finished: map[string]models.Match{}, standing: map[string]standing{}}
	favs, err := repos.ListFavorites(ctx)
	if err != nil {
		return s
	}
	y := now.Year()
	for _, f := range favs {
		if f.Kind != models.FavoriteTeam {
			continue
		}
		matches, err := repos.MatchesForTeam(ctx, f.ID, models.MatchFilter{}, []int{y - 1, y})
		if err == nil {
			for _, m := range matches {
				if m.Finished() && now.Sub(m.Kickoff) < recent {
					s.finished[m.ID] = m
				}
			}
		}
		history, err := repos.TeamHistory(ctx, f.ID)
		if err != nil {
			continue
		}
		for _, row := range history {
			if row.Position == nil {
				continue
			}
			key := f.ID + "|" + row.CompetitionID
			if prev, ok := s.standing[key]; ok && prev.seasonID >= row.SeasonID {
				continue
			}
			s.standing[key] = standing{f.ID, row.CompetitionID, row.SeasonID, *row.Position}
		}
	}
	return s
}

// News is results (oldest first), then table moves. A before never taken
// yields none.
func News(ctx context.Context, repos storage.Repositories, before, after Snapshot) []string {
	if !before.taken || !after.taken {
		return nil
	}
	var results []models.Match
	for id, m := range after.finished {
		if _, ok := before.finished[id]; !ok {
			results = append(results, m)
		}
	}
	slices.SortFunc(results, func(a, b models.Match) int { return a.Kickoff.Compare(b.Kickoff) })

	var moves []standing
	var was []int
	for key, now := range after.standing {
		prev, ok := before.standing[key]
		if ok && prev.seasonID == now.seasonID && prev.pos != now.pos {
			moves = append(moves, now)
			was = append(was, prev.pos)
		}
	}
	if len(results) == 0 && len(moves) == 0 {
		return nil
	}

	var ids []string
	for _, m := range results {
		ids = append(ids, m.HomeTeamID, m.AwayTeamID)
	}
	for _, mv := range moves {
		ids = append(ids, mv.teamID)
	}
	teams, err := repos.GetTeams(ctx, ids)
	if err != nil {
		teams = map[string]models.Team{}
	}
	name := func(id string) string {
		if t, ok := teams[id]; ok && t.Name != "" {
			return t.Name
		}
		return id
	}
	compName := func(id string) string {
		if c, err := repos.GetCompetition(ctx, id); err == nil && c.Name != "" {
			return c.Name
		}
		return id
	}

	var out []string
	for _, m := range results {
		out = append(out, Result(m, name(m.HomeTeamID), name(m.AwayTeamID), compName(m.CompetitionID)))
	}
	order := make([]int, len(moves))
	for i := range order {
		order[i] = i
	}
	// Map order is random; the ticker shouldn't be.
	slices.SortFunc(order, func(a, b int) int { return moves[a].pos - moves[b].pos })
	for _, i := range order {
		mv := moves[i]
		out = append(out, Move(name(mv.teamID), compName(mv.compID), was[i], mv.pos))
	}
	return out
}

// Result is a ticker line like "FT  Arsenal 2–1 Chelsea · Premier League".
func Result(m models.Match, home, away, comp string) string {
	score := fmt.Sprintf("%d–%d", *m.HomeScore, *m.AwayScore)
	state := "FT"
	switch m.Status {
	case models.MatchAET:
		state = "AET"
	case models.MatchPens:
		state = "Pens"
		if m.HomePens != nil && m.AwayPens != nil {
			score += fmt.Sprintf(" (%d–%d)", *m.HomePens, *m.AwayPens)
		}
	}
	return fmt.Sprintf("%s  %s %s %s · %s", state, home, score, away, comp)
}

// Move is a ticker line like "↑ Arsenal up to 2nd in ... (from 4th)".
func Move(team, comp string, from, to int) string {
	arrow, dir := "↑", "up"
	if to > from {
		arrow, dir = "↓", "down"
	}
	return fmt.Sprintf("%s %s %s to %s in %s (from %s)", arrow, team, dir, components.Ordinal(to), comp, components.Ordinal(from))
}

// Feed is the ticker's queue: items cycle one at a time, each for Every,
// until they go stale after Keep.
type Feed struct {
	items []item
	shown int
}

type item struct {
	text  string
	added time.Time
}

const (
	// Every is how long each item stays on the line before the next.
	Every = 6 * time.Second
	// Keep is how long news stays in the feed.
	Keep = 15 * time.Minute
	// most is how many items the feed holds; older ones make room.
	most = 12
)

// Add puts lines at the front of the feed and shows the first.
func (f *Feed) Add(now time.Time, lines ...string) {
	if len(lines) == 0 {
		return
	}
	fresh := make([]item, 0, len(lines)+len(f.items))
	for _, l := range lines {
		fresh = append(fresh, item{l, now})
	}
	f.items = append(fresh, f.items...)
	if len(f.items) > most {
		f.items = f.items[:most]
	}
	f.shown = 0
}

// Advance drops stale items and reports whether anything is left.
func (f *Feed) Advance(now time.Time) bool {
	f.items = slices.DeleteFunc(f.items, func(it item) bool { return now.Sub(it.added) > Keep })
	if len(f.items) == 0 {
		f.shown = 0
		return false
	}
	f.shown = (f.shown + 1) % len(f.items)
	return true
}

// Current is the item on the line with an "i/n" suffix when there are
// several; "" when empty.
func (f *Feed) Current() string {
	if len(f.items) == 0 {
		return ""
	}
	text := f.items[f.shown%len(f.items)].text
	if len(f.items) > 1 {
		text += fmt.Sprintf("  %d/%d", f.shown%len(f.items)+1, len(f.items))
	}
	return text
}

// Len is the number of items in the feed.
func (f *Feed) Len() int { return len(f.items) }
