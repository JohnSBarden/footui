// Package playerstats judges a player by their position: which of the
// four groups they're in, which of their numbers are unusual for it, and
// how a keeper did against the goals they were expected to concede. It is
// pure logic over cached rows, shared by the player page and the
// dashboard's player cards.
package playerstats

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/storage"
)

// Group is a position, as broadly as footui shows one.
type Group int

const (
	GK Group = iota
	DEF
	MID
	FWD
)

// Short is the group's badge: "GK", "DEF", "MID", "FWD".
func (g Group) Short() string { return [...]string{"GK", "DEF", "MID", "FWD"}[g] }

// Singular names one player of the group: "a defender".
func (g Group) Singular() string {
	return [...]string{"keeper", "defender", "midfielder", "forward"}[g]
}

// Plural names the group's players: "top 9% of defenders".
func (g Group) Plural() string { return g.Singular() + "s" }

// GroupOf picks a player's group. ESPN's single position ("G", "D", "M",
// "F") wins when there is one. understat lists every position played,
// alphabetically ("D M S"), so with more than one the numbers decide: a
// player who scores more than they assist and has played up front is a
// forward; otherwise midfield wins if listed, else the first listed.
func GroupOf(espnPos, understatCode string, lines []models.PlayerSeasonStats) Group {
	switch espnPos {
	case "G":
		return GK
	case "D":
		return DEF
	case "M":
		return MID
	case "F":
		return FWD
	}
	has := map[string]bool{}
	var order []string
	for _, f := range strings.Fields(understatCode) {
		if f == "S" || has[f] {
			continue
		}
		has[f] = true
		order = append(order, f)
	}
	if has["GK"] {
		return GK
	}
	goals, assists := 0, 0
	for _, l := range lines {
		goals += l.Goals
		assists += l.Assists
	}
	scorer := goals > assists
	byCode := map[string]Group{"D": DEF, "M": MID, "F": FWD}
	switch {
	case len(order) == 0:
		if scorer {
			return FWD
		}
		return MID
	case len(order) == 1:
		if g, ok := byCode[order[0]]; ok {
			return g
		}
		return MID
	case scorer && has["F"]:
		return FWD
	case has["M"]:
		return MID
	}
	if g, ok := byCode[order[0]]; ok {
		return g
	}
	return MID
}

// Standout is a number unusual for the player's position.
type Standout struct {
	Stat   string // "goals", "assists", "xG", "xA", "shots", "key passes"
	Value  float64
	Per90  float64
	TopPct int // the player is in the top TopPct% of their group
}

const (
	// minMinutes is the playing time below which per-90 rates are noise,
	// for the player and for everyone they're compared with.
	minMinutes = 900
	// minBaseline is the fewest comparable players a percentage means
	// anything against.
	minBaseline = 10
	// standoutPct is how high up their group a player must be.
	standoutPct  = 10
	maxStandouts = 2
)

// statDef reads one candidate stat off a line; ok is false when the line
// doesn't have it (no xG for an ESPN league).
type statDef struct {
	name string
	get  func(models.PlayerSeasonStats) (float64, bool)
}

var candidates = []statDef{
	{"goals", func(l models.PlayerSeasonStats) (float64, bool) { return float64(l.Goals), true }},
	{"assists", func(l models.PlayerSeasonStats) (float64, bool) { return float64(l.Assists), true }},
	{"xG", func(l models.PlayerSeasonStats) (float64, bool) { return deref(l.XG) }},
	{"xA", func(l models.PlayerSeasonStats) (float64, bool) { return deref(l.XA) }},
	{"shots", func(l models.PlayerSeasonStats) (float64, bool) { return derefInt(l.Shots) }},
	{"key passes", func(l models.PlayerSeasonStats) (float64, bool) { return derefInt(l.KeyPasses) }},
}

// own are the stats a group's own tiles already lead with, which can't
// stand out for it: a forward's goals are the point of a forward.
var own = map[Group][]string{
	MID: {"goals", "assists", "key passes"},
	FWD: {"goals", "assists", "shots"},
}

// Standouts are the line's numbers in the top 10% of its group, per 90,
// among the baseline's players with at least 900 minutes: at most two,
// the most unusual first. baseline is every line in the same
// competition-season (the player's included); groups gives each
// baseline player's group.
func Standouts(line models.PlayerSeasonStats, g Group, baseline []models.PlayerSeasonStats, groups map[string]Group) []Standout {
	if line.Minutes < minMinutes {
		return nil
	}
	var out []Standout
	for _, c := range candidates {
		if slices.Contains(own[g], c.name) {
			continue
		}
		v, ok := c.get(line)
		if !ok || v <= 0 {
			continue
		}
		mine := v * 90 / float64(line.Minutes)
		size, better := 0, 0
		for _, b := range baseline {
			bg, known := groups[b.PlayerID]
			if !known || bg != g || b.Minutes < minMinutes {
				continue
			}
			bv, ok := c.get(b)
			if !ok {
				continue
			}
			size++
			if bv*90/float64(b.Minutes) > mine {
				better++
			}
		}
		if size < minBaseline {
			continue
		}
		pct := max(1, int(math.Ceil(100*float64(better)/float64(size))))
		if pct <= standoutPct {
			out = append(out, Standout{Stat: c.name, Value: v, Per90: mine, TopPct: pct})
		}
	}
	slices.SortStableFunc(out, func(a, b Standout) int { return a.TopPct - b.TopPct })
	if len(out) > maxStandouts {
		out = out[:maxStandouts]
	}
	return out
}

// SeasonStandouts compares one season line with every cached line in the
// same competition-season, each player grouped by their listed position.
func SeasonStandouts(ctx context.Context, repos storage.Repositories, line models.PlayerSeasonStats, g Group) ([]Standout, error) {
	if line.Minutes < minMinutes {
		return nil, nil // can't stand out; spare the queries
	}
	lines, err := repos.SeasonPlayerLines(ctx, line.CompetitionID, line.SeasonID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(lines))
	byPlayer := map[string][]models.PlayerSeasonStats{}
	for _, l := range lines {
		if _, seen := byPlayer[l.PlayerID]; !seen {
			ids = append(ids, l.PlayerID)
		}
		byPlayer[l.PlayerID] = append(byPlayer[l.PlayerID], l)
	}
	players, err := repos.GetPlayers(ctx, ids)
	if err != nil {
		return nil, err
	}
	groups := make(map[string]Group, len(players))
	for id, p := range players {
		groups[id] = GroupOf("", p.Position, byPlayer[id])
	}
	groups[line.PlayerID] = g
	return Standouts(line, g, lines, groups), nil
}

// sparseShare is how far below a player's own best defensive rate a
// season must fall to count as barely recorded rather than quiet.
const sparseShare = 0.25

// ClearSparseDefence blanks the tackles, interceptions and clearances of
// seasons the source barely recorded. ESPN's older seasons often carry a
// token handful (Rice's 2024-25: 18 in 2876 minutes, against 185 the next
// season), which reads as a player who stopped defending. A season under
// a quarter of the player's own best rate, both over 900 minutes, is
// treated as not recorded. A player quiet every season keeps his
// numbers: it's the drop against himself that gives a season away.
func ClearSparseDefence(details []*models.PlayerSeasonDetail) {
	rate := func(d *models.PlayerSeasonDetail) (float64, bool) {
		if d.Minutes == nil || *d.Minutes < minMinutes || d.Tackles == nil || d.Interceptions == nil || d.Clearances == nil {
			return 0, false
		}
		return float64(*d.Tackles+*d.Interceptions+*d.Clearances) * 90 / float64(*d.Minutes), true
	}
	best := 0.0
	for _, d := range details {
		if r, ok := rate(d); ok {
			best = max(best, r)
		}
	}
	for _, d := range details {
		if r, ok := rate(d); ok && r < best*sparseShare {
			d.Tackles, d.Interceptions, d.Clearances = nil, nil, nil
		}
	}
}

// KeeperExpected estimates the goals a keeper was expected to concede: his
// club's xGA, scaled by his share of its minutes. No source has xG faced
// per keeper, so this is labelled an estimate wherever it's shown. ok is
// false without the club's xGA or matches.
func KeeperExpected(minutes int, team models.TeamSeasonStats) (float64, bool) {
	if team.XGA == nil || team.MatchesPlayed == 0 {
		return 0, false
	}
	share := float64(minutes) / float64(team.MatchesPlayed*90)
	return *team.XGA * min(share, 1), true
}

func deref(p *float64) (float64, bool) {
	if p == nil {
		return 0, false
	}
	return *p, true
}

func derefInt(p *int) (float64, bool) {
	if p == nil {
		return 0, false
	}
	return float64(*p), true
}
