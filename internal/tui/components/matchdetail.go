package components

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/data/models"
	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// Event glyphs, shared by the timeline axis, the timeline rows and their
// key. Each is one cell wide.
const (
	GlyphGoal = "●"
	GlyphCard = "■"
	GlyphSub  = "▲"
	GlyphMiss = "○"
)

// EventGlyph is the event's mark in the theme's colours: goals and red
// cards stand out, yellows read as a booking, subs stay quiet.
func EventGlyph(th styles.Theme, e models.MatchEvent) string {
	switch {
	case e.IsGoal():
		return th.Positive.Bold(true).Render(GlyphGoal)
	case e.Kind == models.EventPenaltyMiss:
		return th.Negative.Render(GlyphMiss)
	case e.Kind == models.EventYellow:
		return th.Draw.Render(GlyphCard)
	case e.IsCard():
		return th.Negative.Bold(true).Render(GlyphCard)
	case e.Kind == models.EventSub:
		return th.Muted.Render(GlyphSub)
	}
	return ""
}

// eventRank orders what wins a shared cell on the axis.
func eventRank(e models.MatchEvent) int {
	switch {
	case e.IsGoal():
		return 4
	case e.Kind == models.EventRed || e.Kind == models.EventSecondYellow:
		return 3
	case e.Kind == models.EventPenaltyMiss:
		return 2
	case e.Kind == models.EventYellow:
		return 1
	case e.Kind == models.EventSub:
		return 0
	}
	return -1
}

// timelineLength is the minutes the axis spans: 90, or 120 after extra
// time.
func timelineLength(events []models.MatchEvent) int {
	for _, e := range events {
		if e.Period >= 3 || e.Minute > 90 {
			return 120
		}
	}
	return 90
}

// Timeline draws the match on one line, width cells wide: a minute axis
// with each event at its minute, side's in the accent and the opponent's
// muted, half time as ┼. side is the team the screen is about ("home" or
// "away"); "" colours both sides alike.
func Timeline(th styles.Theme, events []models.MatchEvent, side string, width int) string {
	length := timelineLength(events)
	start, end := "0'", itoa(length)+"'"
	axis := width - lipgloss.Width(start) - lipgloss.Width(end) - 2
	if axis < 10 {
		return ""
	}
	type mark struct {
		e    models.MatchEvent
		rank int
	}
	cells := make([]*mark, axis)
	at := func(minute int) int {
		return min(max(int(math.Round(float64(minute)/float64(length)*float64(axis-1))), 0), axis-1)
	}
	half := -1
	for _, e := range events {
		if e.Kind == models.EventHalfTime && e.Period == 1 {
			half = at(45)
			continue
		}
		r := eventRank(e)
		if r < 0 || e.Period == 5 {
			continue
		}
		i := at(e.Minute)
		if cells[i] == nil || r > cells[i].rank {
			cells[i] = &mark{e, r}
		}
	}

	var b strings.Builder
	b.WriteString(th.Muted.Render(start + " "))
	for i, c := range cells {
		switch {
		case c != nil:
			b.WriteString(sideGlyph(th, c.e, side))
		case i == half:
			b.WriteString(th.Faint.Render("┼"))
		default:
			b.WriteString(th.Faint.Render("─"))
		}
	}
	b.WriteString(th.Muted.Render(" " + end))
	return b.String()
}

// sideGlyph keeps goals and reds in their own colours whoever they're
// for, and mutes the opponent's bookings and subs. Without colour the
// screen's own side is told apart by case: the opponent's are lower.
func sideGlyph(th styles.Theme, e models.MatchEvent, side string) string {
	ours := side == "" || e.Side == side
	if th.NoColor {
		g := map[int]string{4: "G", 3: "R", 2: "M", 1: "Y", 0: "S"}[eventRank(e)]
		if !ours {
			g = strings.ToLower(g)
		}
		return g
	}
	if !ours && !e.IsGoal() && eventRank(e) < 3 {
		return th.Faint.Render(glyphFor(e))
	}
	if !ours && e.IsGoal() {
		return th.Negative.Bold(true).Render(GlyphGoal)
	}
	return EventGlyph(th, e)
}

func glyphFor(e models.MatchEvent) string {
	switch {
	case e.IsGoal():
		return GlyphGoal
	case e.Kind == models.EventPenaltyMiss:
		return GlyphMiss
	case e.IsCard():
		return GlyphCard
	}
	return GlyphSub
}

// SplitBar compares two values on one bar width cells wide, the first
// side's share filled from the left: "████████░░░░░░░░". Both nil or zero
// draws nothing.
func SplitBar(th styles.Theme, a, b float64, width int) string {
	if width <= 0 || a < 0 || b < 0 || a+b == 0 {
		return ""
	}
	left := int(math.Round(a / (a + b) * float64(width)))
	if th.NoColor {
		return strings.Repeat("#", left) + strings.Repeat(".", width-left)
	}
	return th.Accent.Render(strings.Repeat("█", left)) + th.Faint.Render(strings.Repeat("░", width-left))
}

// Scorers lists one side's goals the way a report does, "Havertz 16',
// Saka 54' (pen)", with own goals marked; the opponent's own goals count
// for this side.
func Scorers(events []models.MatchEvent, side string) string {
	var parts []string
	for _, e := range events {
		if !e.IsGoal() || e.Side != side || e.Period == 5 {
			continue
		}
		s := ShortName(e.Player) + " " + e.Clock()
		switch e.Kind {
		case models.EventPenaltyGoal:
			s += " (pen)"
		case models.EventOwnGoal:
			s += " (og)"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

// ShortName is the surname a match report uses: "Kai Havertz" is
// "Havertz", "Virgil van Dijk" is "van Dijk"; a single name is kept.
func ShortName(name string) string {
	words := strings.Fields(name)
	if len(words) <= 1 {
		return name
	}
	// Keep the particles before the surname, but never the first name.
	i := len(words) - 1
	for i > 1 && isParticle(words[i-1]) {
		i--
	}
	return strings.Join(words[i:], " ")
}

func isParticle(w string) bool {
	switch strings.ToLower(w) {
	case "van", "von", "de", "der", "den", "da", "di", "del", "la", "le", "dos", "das", "ter", "el", "al", "bin":
		return true
	}
	return false
}
