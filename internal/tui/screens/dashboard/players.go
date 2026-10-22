package dashboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/data/playerstats"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/core"
)

// playerDetail fills a player card's position, its season detail, a
// keeper's xGA estimate and the season's standouts. A card without detail
// keeps its goals-and-assists lines, so failures here only cost the extra.
func playerDetail(deps core.Deps, c *Card, history []models.PlayerSeasonStats) {
	s := c.PlayerStats
	details, err := deps.Repos.PlayerSeasonDetails(deps.Ctx, c.Player.ID)
	if err != nil {
		return
	}
	detailPos := ""
	var sameComp []*models.PlayerSeasonDetail
	for i := range details {
		d := &details[i]
		playerstats.ScrubUnrecorded(d)
		if detailPos == "" {
			detailPos = d.Position
		}
		if d.CompetitionID == s.CompetitionID {
			sameComp = append(sameComp, d)
		}
		if d.TeamID == s.TeamID && d.CompetitionID == s.CompetitionID && d.SeasonID == s.SeasonID {
			c.Detail = d
		}
	}
	// A season barely recorded next to the player's best reads as
	// unrecorded, as on the player page.
	playerstats.ClearSparseDefence(sameComp)
	c.Position = playerstats.GroupOf(detailPos, c.Player.Position, history)
	if c.Detail == nil {
		return
	}
	if standouts, err := playerstats.SeasonStandouts(deps.Ctx, deps.Repos, *s, c.Position); err == nil {
		c.Standouts = standouts
	}
	if c.Position != playerstats.GK {
		return
	}
	club, err := deps.Repos.TeamHistory(deps.Ctx, s.TeamID)
	if err != nil {
		return
	}
	for _, ts := range club {
		if ts.CompetitionID == s.CompetitionID && ts.SeasonID == s.SeasonID {
			if xga, ok := playerstats.KeeperExpected(detailMinutes(c), ts); ok {
				c.XGA = &xga
			}
		}
	}
}

func detailMinutes(c *Card) int {
	if c.Detail.Minutes != nil && *c.Detail.Minutes > 0 {
		return *c.Detail.Minutes
	}
	return c.PlayerStats.Minutes
}

func detailApps(c Card) int {
	if c.Detail.Appearances != nil && *c.Detail.Appearances > 0 {
		return *c.Detail.Appearances
	}
	return c.PlayerStats.Appearances
}

func rate(n *int, minutes int) string {
	if n == nil || minutes < 90 {
		return "—"
	}
	return strconv.FormatFloat(float64(*n)*90/float64(minutes), 'f', 2, 64)
}

func share(num, den int) string {
	if den <= 0 {
		return "—"
	}
	return strconv.Itoa(int(float64(num)*100/float64(den)+0.5)) + "%"
}

// positionLines are a player card's numbers for their position, under
// the heading and the club line: a headline, one or two stat lines, and a
// standout when the season has one.
func (m *Model) positionLines(c Card, inner int) []string {
	th := m.deps.Theme
	d, s := c.Detail, c.PlayerStats
	mins := detailMinutes(&c)
	// The line's own counts (shots, key passes) go over the line's own
	// minutes; an ESPN league's line has none, so the detail's stand in.
	lineMins := s.Minutes
	if lineMins < 90 {
		lineMins = mins
	}
	var headline string
	var rest []string
	switch c.Position {
	case playerstats.GK:
		headline = components.Plural(deref(d.CleanSheets), "clean sheet") + " · " + strconv.Itoa(deref(d.GoalsConceded)) + " conceded"
		if d.Saves != nil && d.GoalsConceded != nil {
			rest = append(rest, components.Plural(*d.Saves, "save")+" · "+share(*d.Saves, *d.Saves+*d.GoalsConceded)+" saved")
		}
		if c.XGA != nil && d.GoalsConceded != nil {
			rest = append(rest, components.SignedFloat(*c.XGA-float64(*d.GoalsConceded), 1)+" vs expected (est.)")
		}
	case playerstats.DEF:
		headline = components.Plural(deref(d.CleanSheets), "clean sheet") + " · " + components.Plural(detailApps(c), "app")
		if d.Tackles != nil {
			rest = append(rest, rate(d.Tackles, mins)+" tkl · "+rate(d.Interceptions, mins)+" int per 90")
		}
	case playerstats.MID:
		headline = components.Plural(s.Goals, "goal") + " · " + components.Plural(s.Assists, "assist")
		kp := ""
		if s.KeyPasses != nil {
			kp = rate(s.KeyPasses, lineMins) + " KP/90"
		}
		if line := joinNonEmpty(xParts(s), kp); line != "" {
			rest = append(rest, line)
		}
		if d.Tackles != nil && d.Interceptions != nil {
			won := *d.Tackles + *d.Interceptions
			line := rate(&won, mins) + " tkl+int/90"
			if d.PassPct != nil {
				line += " · " + strconv.Itoa(int(*d.PassPct*100+0.5)) + "% pass"
			}
			rest = append(rest, line)
		}
	default:
		headline = components.Plural(s.Goals, "goal") + " · " + components.Plural(s.Assists, "assist")
		from := ""
		if s.XG != nil {
			from = "from " + components.Float(s.XG, 1) + " xG"
		}
		// A rate needs minutes; without any (ESPN's older seasons) the
		// shot total stands in.
		shots := ""
		switch {
		case s.Shots != nil && lineMins >= 90:
			shots = rate(s.Shots, lineMins) + " shots/90"
		case s.Shots != nil:
			shots = components.Plural(*s.Shots, "shot") + " · " + components.Plural(detailApps(c), "app")
		}
		if line := joinNonEmpty(from, shots); line != "" {
			rest = append(rest, line)
		}
		played := ""
		if mins >= 90 {
			played = strconv.Itoa(mins) + " min"
		}
		switch {
		case d.ShotsOnTarget != nil && s.Shots != nil && *s.Shots > 0:
			rest = append(rest, joinNonEmpty(share(*d.ShotsOnTarget, *s.Shots)+" on target", played))
		case played != "":
			rest = append(rest, played+" · "+components.Plural(detailApps(c), "app"))
		}
	}

	// The position rides at the right of the headline, so the club line
	// above keeps its season.
	chip := c.Position.Short()
	head := th.Accent.Bold(true).Render(components.Truncate(headline, inner-len(chip)-1))
	gap := max(inner-lipgloss.Width(head)-len(chip), 1)
	lines := []string{head + strings.Repeat(" ", gap) + th.Muted.Render(chip)}
	for _, r := range rest {
		lines = append(lines, th.TableCell.Render(components.Truncate(r, inner)))
	}
	if len(c.Standouts) > 0 {
		st := c.Standouts[0]
		value := strconv.Itoa(int(st.Value))
		if st.Stat == "xG" || st.Stat == "xA" {
			value = strconv.FormatFloat(st.Value, 'f', 1, 64)
		}
		lines = append(lines, th.Accent.Render("▲ ")+th.TableCell.Render(components.Truncate(
			fmt.Sprintf("%s %s, top %d%% of %s", value, st.Stat, st.TopPct, c.Position.Plural()), inner-2)))
	}
	return lines
}

func xParts(s *models.PlayerSeasonStats) string {
	var parts []string
	if s.XG != nil {
		parts = append(parts, "xG "+components.Float(s.XG, 1))
	}
	if s.XA != nil {
		parts = append(parts, "xA "+components.Float(s.XA, 1))
	}
	return strings.Join(parts, " · ")
}

func joinNonEmpty(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func deref(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}
