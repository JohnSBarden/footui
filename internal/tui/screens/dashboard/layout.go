package dashboard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Card geometry. Cards share the screen's width in equal columns at least
// minCardWidth wide, and are as tall as what they hold. A league's card
// spans two columns and is leagueHeight tall — twice a typical card each
// way — so its table has room.
const (
	minCardWidth = 36
	cardGap      = 1
	leagueHeight = 20
	// headerLines is the dashboard's own title and the blank line under it.
	headerLines = 2
)

// slot is where one card sits: its column, how many columns it spans,
// and the lines it covers.
type slot struct {
	card, col, span, top, height int
	lines                        []string
}

func (s slot) bottom() int { return s.top + s.height }

// columns is how many columns fit side by side, and how wide each is.
func (m *Model) columns() (cols, width int) {
	cols = max((m.width+cardGap)/(minCardWidth+cardGap), 1)
	width = (m.width - cardGap*(cols-1)) / cols
	return cols, max(width, 12)
}

// span is how many columns a card takes.
func (m *Model) span(c Card, cols int) int {
	if cols >= 2 && c.Group == GroupLeagues && !c.Missing {
		return 2
	}
	return 1
}

// layout flows the cards, in order, into the columns: each goes wherever
// it can sit highest, leftmost on a tie, so the cards pack together
// instead of lining up in rows with gaps under the short ones.
func (m *Model) layout() []slot {
	cols, width := m.columns()
	heights := make([]int, cols)
	slots := make([]slot, 0, len(m.cards))
	for i, c := range m.cards {
		span := m.span(c, cols)
		w := width*span + cardGap*(span-1)
		lines := strings.Split(m.renderCard(c, i == m.cursor, w), "\n")
		best, top := 0, -1
		for col := 0; col+span <= cols; col++ {
			y := 0
			for _, h := range heights[col : col+span] {
				y = max(y, h)
			}
			if top < 0 || y < top {
				best, top = col, y
			}
		}
		for col := best; col < best+span; col++ {
			heights[col] = top + len(lines)
		}
		slots = append(slots, slot{card: i, col: best, span: span, top: top, height: len(lines), lines: lines})
	}
	return slots
}

// cardsView draws the packed cards, scrolled so the selected one is on
// screen.
func (m *Model) cardsView() string {
	cols, width := m.columns()
	slots := m.layout()
	total := 0
	for _, s := range slots {
		total = max(total, s.bottom())
	}
	// owner[y][col] is the slot covering that cell, -1 for none.
	owner := make([][]int, total)
	for y := range owner {
		owner[y] = make([]int, cols)
		for col := range owner[y] {
			owner[y][col] = -1
		}
	}
	for i, s := range slots {
		for y := s.top; y < s.bottom(); y++ {
			for col := s.col; col < s.col+s.span; col++ {
				owner[y][col] = i
			}
		}
	}

	budget := max(m.height-headerLines-m.stripLines(), 1)
	first := 0
	if sel, ok := slotOf(slots, m.cursor); ok && sel.bottom() > budget {
		first = min(sel.bottom()-budget, sel.top)
	}
	blank := strings.Repeat(" ", width)
	var out []string
	for y := first; y < total && y < first+budget; y++ {
		var segs []string
		for col := 0; col < cols; {
			i := owner[y][col]
			if i < 0 {
				segs = append(segs, blank)
				col++
				continue
			}
			s := slots[i]
			line := s.lines[y-s.top]
			w := width*s.span + cardGap*(s.span-1)
			segs = append(segs, line+strings.Repeat(" ", max(w-lipgloss.Width(line), 0)))
			col += s.span
		}
		out = append(out, strings.TrimRight(strings.Join(segs, strings.Repeat(" ", cardGap)), " "))
	}
	return strings.Join(out, "\n")
}

// stripLines are the lines the scores along the top take, with the blank
// line under them.
func (m *Model) stripLines() int {
	if m.strip.Empty() {
		return 0
	}
	return m.strip.Height() + 1
}

func slotOf(slots []slot, card int) (slot, bool) {
	for _, s := range slots {
		if s.card == card {
			return s, true
		}
	}
	return slot{}, false
}

// move shifts the cursor to the nearest card in a direction, by where the
// cards sit on screen: up and down stay in the selected card's columns,
// left and right prefer the card beside it, level with it.
func (m *Model) move(dy, dx int) {
	slots := m.layout()
	cur, ok := slotOf(slots, m.cursor)
	if !ok {
		return
	}
	best := -1
	var bestKey [3]int
	for i, s := range slots {
		var key [3]int
		switch {
		case dy > 0 && overlaps(s.col, s.col+s.span, cur.col, cur.col+cur.span) && s.top >= cur.bottom():
			key = [3]int{s.top - cur.bottom(), abs(s.col - cur.col), s.top}
		case dy < 0 && overlaps(s.col, s.col+s.span, cur.col, cur.col+cur.span) && s.bottom() <= cur.top:
			key = [3]int{cur.top - s.bottom(), abs(s.col - cur.col), -s.top}
		case dx > 0 && s.col >= cur.col+cur.span:
			key = [3]int{s.col - cur.col - cur.span, beside(s, cur), abs(s.top - cur.top)}
		case dx < 0 && s.col+s.span <= cur.col:
			key = [3]int{cur.col - s.col - s.span, beside(s, cur), abs(s.top - cur.top)}
		default:
			continue
		}
		if best < 0 || less(key, bestKey) {
			best, bestKey = i, key
		}
	}
	if best >= 0 {
		m.cursor = slots[best].card
	}
}

func overlaps(a0, a1, b0, b1 int) bool { return a0 < b1 && b0 < a1 }

// beside is 0 when two cards share a line on screen, 1 when they don't.
func beside(a, b slot) int {
	if overlaps(a.top, a.bottom(), b.top, b.bottom()) {
		return 0
	}
	return 1
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
