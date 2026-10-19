package scorestrip

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/components"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Lines is how tall a strip with any games in it is: the day, then each
// tile's competition and state over its two sides.
const Lines = 4

// Tile geometry. A tile is at least minTile wide and at most maxTile;
// between tiles is a rule, and the strip keeps a column either side for
// the arrows that say more games are off screen.
const (
	minTile = 17
	maxTile = 24
	sep     = " │ "
	edge    = 2
)

// Strip is a row of games, one tile each, scrolled sideways.
type Strip struct {
	// Matches are in kickoff order.
	Matches []models.Match
	Teams   map[string]models.Team
	Comps   map[string]models.Competition
	Colors  map[string]models.TeamColors
	// ShowComp names each tile's competition; a league's own strip, where
	// every game is in it, leaves it out.
	ShowComp bool
	// Highlight picks out a team's name when it reports true for it: the
	// starred clubs on a league's strip.
	Highlight func(teamID string) bool
	// Now is what "today" and "in play" are measured against.
	Now time.Time

	// first is the leftmost tile shown once the strip has been scrolled;
	// until then it opens on the games in play and next up.
	first    int
	scrolled bool
}

// Empty reports whether there is nothing to draw.
func (s Strip) Empty() bool { return len(s.Matches) == 0 }

// Height is the lines the strip takes, none when it's empty.
func (s Strip) Height() int {
	if s.Empty() {
		return 0
	}
	return Lines
}

// fit is how many tiles fit in width, and how wide each is.
func fit(width int) (n, tile int) {
	room, gap := width-2*edge, lipgloss.Width(sep)
	n = max((room+gap)/(minTile+gap), 1)
	tile = (room - gap*(n-1)) / n
	return n, min(max(tile, 8), maxTile)
}

// start is the leftmost tile shown for n tiles: where it was scrolled to,
// or else with the games in play and to come filling the right third, and
// the latest results the rest.
func (s Strip) start(n int) int {
	last := max(len(s.Matches)-n, 0)
	if s.scrolled {
		return min(s.first, last)
	}
	pivot := len(s.Matches)
	for i, m := range s.Matches {
		if current(m, s.Now) {
			pivot = i
			break
		}
	}
	ahead := min(len(s.Matches)-pivot, max((n+1)/3, 1))
	return min(max(pivot+ahead-n, 0), last)
}

// Scroll moves the strip a page of tiles left (dir < 0) or right.
func (s *Strip) Scroll(dir, width int) {
	n, _ := fit(width)
	first := s.start(n) + dir*n
	s.first = min(max(first, 0), max(len(s.Matches)-n, 0))
	s.scrolled = true
}

// View draws the strip width columns wide, or nothing when it's empty.
func (s Strip) View(th styles.Theme, width int) string {
	if s.Empty() {
		return ""
	}
	n, tile := fit(width)
	first := s.start(n)
	shown := s.Matches[first:min(first+n, len(s.Matches))]

	rows := make([][]string, Lines)
	prevDay := ""
	for i, m := range shown {
		day := s.day(m.Kickoff)
		label := ""
		if i == 0 || day != prevDay {
			label = day
		}
		prevDay = day
		cells := []string{s.dayCell(th, label, tile), s.stateLine(th, m, tile),
			s.side(th, m, m.HomeTeamID, tile), s.side(th, m, m.AwayTeamID, tile)}
		for r, c := range cells {
			if i > 0 {
				gap := th.Faint.Render(sep)
				if r == 0 {
					gap = strings.Repeat(" ", lipgloss.Width(sep))
				}
				rows[r] = append(rows[r], gap)
			}
			rows[r] = append(rows[r], pad(c, tile))
		}
	}

	left, right := "  ", "  "
	if first > 0 {
		left = th.Accent.Render("‹") + " "
	}
	if first+len(shown) < len(s.Matches) {
		right = " " + th.Accent.Render("›")
	}
	out := make([]string, Lines)
	for r, cells := range rows {
		l, rt := "  ", ""
		if r == 2 {
			l, rt = left, right
		}
		out[r] = strings.TrimRight(l+strings.Join(cells, "")+rt, " ")
	}
	return strings.Join(out, "\n")
}

// day is the day a game is on, as the strip heads it: "Today",
// "Yesterday", "Tomorrow", or "Sat 17 Oct".
func (s Strip) day(t time.Time) string {
	date := func(t time.Time) time.Time {
		y, m, d := t.Local().Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	today, k := date(s.Now), date(t)
	switch {
	case k.Equal(today):
		return "Today"
	case k.Equal(today.AddDate(0, 0, -1)):
		return "Yesterday"
	case k.Equal(today.AddDate(0, 0, 1)):
		return "Tomorrow"
	}
	return t.Local().Format("Mon 2 Jan")
}

func (s Strip) dayCell(th styles.Theme, label string, tile int) string {
	if label == "Today" {
		return th.Accent.Bold(true).Render(label)
	}
	return th.Label.Render(components.Truncate(label, tile))
}

// stateLine is the competition, on the left when the strip shows it, and
// where the game is on the right: "FT", "LIVE", "19:45".
func (s Strip) stateLine(th styles.Theme, m models.Match, tile int) string {
	state, style := s.state(th, m)
	comp := ""
	if s.ShowComp {
		c, ok := s.Comps[m.CompetitionID]
		if !ok {
			c = models.Competition{ID: m.CompetitionID, Name: m.CompetitionID}
		}
		comp = components.Truncate(c.Abbrev(), max(tile-lipgloss.Width(state)-1, 1))
	}
	gap := max(tile-lipgloss.Width(comp)-lipgloss.Width(state), 1)
	return th.Muted.Render(comp) + strings.Repeat(" ", gap) + style.Render(state)
}

// state is a game's state as its tile shows it, and how it's drawn.
func (s Strip) state(th styles.Theme, m models.Match) (string, lipgloss.Style) {
	switch m.Status {
	case models.MatchPostponed:
		return "PPD", th.Muted
	case models.MatchCancelled:
		return "CANC", th.Muted
	case models.MatchInPlay:
		return "● LIVE", th.Accent.Bold(true)
	case models.MatchAET:
		return "AET", th.Muted
	case models.MatchPens:
		if m.HomePens != nil && m.AwayPens != nil {
			return "PENS " + strconv.Itoa(*m.HomePens) + "–" + strconv.Itoa(*m.AwayPens), th.Muted
		}
		return "PENS", th.Muted
	}
	switch {
	case m.Finished():
		return "FT", th.Muted
	case !m.Kickoff.After(s.Now):
		// Kicked off, but no result is stored yet: a refresh brings it.
		return "awaiting", th.Faint
	}
	return m.Kickoff.Local().Format("15:04"), th.TableCell
}

// side is one team's line on a tile: its colour dot, its name and its
// goals, the winner bright and the loser dimmed.
func (s Strip) side(th styles.Theme, m models.Match, teamID string, tile int) string {
	team, ok := s.Teams[teamID]
	name := teamID
	if ok {
		name = team.ShortName
		if name == "" {
			name = team.Name
		}
	}
	goals := ""
	if scored, _, _ := m.For(teamID); scored != nil && m.Status != models.MatchPostponed && m.Status != models.MatchCancelled {
		goals = strconv.Itoa(*scored)
	}
	style := th.TableCell
	switch m.Result(teamID) {
	case "W":
		style = th.TableCell.Bold(true)
	case "L":
		style = th.Muted
	}
	if m.Status == models.MatchPens && m.Result(teamID) == "D" && m.HomePens != nil && m.AwayPens != nil {
		ours, theirs := *m.HomePens, *m.AwayPens
		if teamID == m.AwayTeamID {
			ours, theirs = theirs, ours
		}
		if ours > theirs {
			style = th.TableCell.Bold(true)
		} else {
			style = th.Muted
		}
	}
	scoreStyle := style
	if m.Status == models.MatchInPlay {
		scoreStyle = th.Accent.Bold(true)
	}
	nameStyle := style
	if s.Highlight != nil && s.Highlight(teamID) {
		nameStyle = th.Accent.Bold(true)
	}
	name = shorten(name, tile-2-1-max(lipgloss.Width(goals), 2))
	return components.Dot(th, s.Colors[teamID].PrimaryHex) + " " +
		nameStyle.Render(name) + strings.Repeat(" ", max(tile-2-lipgloss.Width(name)-lipgloss.Width(goals), 1)) +
		scoreStyle.Render(goals)
}

// shorten fits a name in room the way a scoreboard would, its leading
// words cut to initials before its last is cut short — "M. United" and
// "M. City" rather than "Manchester…" twice.
func shorten(name string, room int) string {
	words := strings.Fields(name)
	for i := 0; lipgloss.Width(name) > room && i < len(words)-1; i++ {
		words[i] = string([]rune(words[i])[:1]) + "."
		name = strings.Join(words, " ")
	}
	return components.Truncate(name, room)
}

// pad fills a cell out to width so the next tile lines up.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}
