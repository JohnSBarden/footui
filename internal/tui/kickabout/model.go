package kickabout

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

const (
	fps        = 30
	roundTicks = 60 * fps
	shotSpeed  = 3.0
	trailLen   = 6
	toastTicks = fps
	goalSize   = 5
	// minW and minH are the smallest pitch worth playing on.
	minW, minH = 60, 20
	// stripe is the width of a mown stripe of grass.
	stripe = 6
)

// TickMsg advances the game; gen ties it to the round that asked for it,
// so a tick still in flight from a finished round is ignored.
type TickMsg struct{ gen int }

// Model is one kickabout over a snapshot of a screen.
type Model struct {
	plain  bool
	paint  [3][kinds]lipgloss.Style
	rng    *rand.Rand
	f      Field
	player Vec
	facing Vec
	ball   Ball
	// dribbling is set while the ball is at the player's feet.
	dribbling bool
	defs      []Defender
	goal      Goal
	party     party
	goals     int
	shots     int
	best      int
	left      int // ticks left in the round
	trail     []Vec
	toast     string
	toastLeft int
	gen       int
	over      bool
	done      bool
	tooSmall  bool
}

// New starts a round over frame, a width×height screen.
func New(frame string, width, height int, th styles.Theme) Model {
	m := Model{plain: th.NoColor, gen: 1}
	if width < minW || height < minH {
		m.tooSmall = true
		return m
	}
	seed := uint64(time.Now().UnixNano())
	m.rng = rand.New(rand.NewPCG(seed, seed>>1))
	m.f = NewField(frame, width, height)
	m.paint = palette()
	m.defs = pickDefenders(m.f, m.kickOffSpot(), m.rng)
	m.reset()
	return m
}

func (m *Model) reset() {
	m.kickOff()
	m.goal = Goal{Top: m.f.H/2 - goalSize/2, Size: goalSize, Dir: 1}
	m.goals, m.shots, m.left, m.over = 0, 0, roundTicks, false
	m.toast, m.toastLeft = "", 0
}

func (m Model) kickOffSpot() Vec { return Vec{2, float64(m.f.H / 2)} }

// kickOff lines everyone up again: the player on the spot with the ball,
// the defenders back home.
func (m *Model) kickOff() {
	m.player = m.kickOffSpot()
	m.facing = Vec{1, 0}
	m.ball = Ball{Pos: m.feet()}
	m.dribbling = true
	m.trail = nil
	m.party = party{}
	for i := range m.defs {
		m.defs[i].Pos, m.defs[i].rest = m.defs[i].Home, 0
	}
}

// feet is where the ball sits while the player dribbles it.
func (m Model) feet() Vec { return Vec{m.player.X + m.facing.X, m.player.Y + m.facing.Y} }

// Done reports that the player pressed esc.
func (m Model) Done() bool { return m.done }

// Init starts the clock.
func (m Model) Init() tea.Cmd {
	if m.tooSmall {
		return nil
	}
	return m.tick()
}

func (m Model) tick() tea.Cmd {
	gen := m.gen
	return tea.Tick(time.Second/fps, func(time.Time) tea.Msg { return TickMsg{gen: gen} })
}

// moves are the keys that run, as a step: two cells across for every one
// down, since cells are about half as wide as they are tall.
var moves = map[string]Vec{
	"up": {0, -1}, "k": {0, -1}, "down": {0, 1}, "j": {0, 1},
	"left": {-2, 0}, "h": {-2, 0}, "right": {2, 0}, "l": {2, 0},
}

// Update handles keys and ticks.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.key(msg)
	case TickMsg:
		if msg.gen != m.gen || m.tooSmall || m.over || m.done {
			return m, nil
		}
		m.step()
		return m, m.tick()
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.done = true
		return m, nil
	case "enter":
		if m.over {
			m.best = max(m.best, m.goals)
			m.gen++
			m.reset()
			return m, m.tick()
		}
		return m, nil
	case " ":
		switch {
		case m.party.on():
			m.kickOff()
		case m.dribbling && !m.over && !m.tooSmall:
			m.dribbling = false
			m.shots++
			m.ball.Vel = Vec{m.facing.X * shotSpeed * 2, m.facing.Y * shotSpeed}
		}
		return m, nil
	}
	d, ok := moves[msg.String()]
	if !ok || m.over || m.tooSmall || m.party.on() {
		return m, nil
	}
	m.facing = Vec{sign(d.X), sign(d.Y)}
	m.player = MovePlayer(m.player, d, m.f, m.blocksPlayer)
	if m.dribbling {
		if feet := m.feet(); m.f.inside(cell(feet.X), cell(feet.Y)) {
			m.ball.Pos = feet
		}
	} else {
		m.pickUp()
	}
	return m, nil
}

func sign(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// blocksPlayer reports a defender on (x, y): the player has to go round.
// One already on top of the player doesn't hold them in place.
func (m Model) blocksPlayer(x, y int) bool {
	px, py := cell(m.player.X), cell(m.player.Y)
	for _, d := range m.defs {
		if d.Covers(x, y) && !d.Covers(px, py) {
			return true
		}
	}
	return false
}

// blocksBall reports a defender on (x, y) the ball bounces off: not one
// still resting after a tackle, nor one the ball is inside already, so a
// cleared ball can roll free.
func (m Model) blocksBall(x, y int) bool {
	bx, by := cell(m.ball.Pos.X), cell(m.ball.Pos.Y)
	for _, d := range m.defs {
		if d.rest == 0 && d.Covers(x, y) && !d.Covers(bx, by) {
			return true
		}
	}
	return false
}

// pickUp takes a stopped ball back to the player's feet once they reach
// it.
func (m *Model) pickUp() {
	near := math.Abs(m.ball.Pos.X-m.player.X) <= 2 && math.Abs(m.ball.Pos.Y-m.player.Y) <= 1
	if near && m.ball.Vel == (Vec{}) {
		m.dribbling = true
		m.ball = Ball{Pos: m.feet()}
		m.trail = nil
	}
}

func (m *Model) step() {
	if m.party.on() {
		m.party.advance(m.f, m.rng)
		if !m.party.on() {
			m.kickOff()
		}
		return
	}
	m.left--
	if m.left <= 0 {
		m.over = true
		return
	}
	m.goal = m.goal.Advance(m.f.H)
	if m.toastLeft > 0 {
		m.toastLeft--
	}
	moveDefenders(m.defs, m.ball.Pos, m.f)
	m.challenge()
	if m.dribbling {
		return
	}
	m.trail = append(m.trail, m.ball.Pos)
	if len(m.trail) > trailLen {
		m.trail = m.trail[1:]
	}
	next, hit, scored := Step(m.ball, m.f, m.blocksBall, m.goal.Open)
	if scored {
		m.goals++
		m.party = party{left: partyTicks, row: cell(next.Pos.Y), goals: m.goals}
		m.toastLeft, m.trail = 0, nil
		return
	}
	if hit != nil {
		if d := m.defenderAt(hit[0], hit[1]); d != nil {
			m.say(fmt.Sprintf("blocked by %q", d.Name))
		}
	}
	m.ball = next
	m.pickUp()
}

func (m *Model) defenderAt(x, y int) *Defender {
	for i := range m.defs {
		if m.defs[i].Covers(x, y) {
			return &m.defs[i]
		}
	}
	return nil
}

// challenge lets a defender that has reached the ball win it: off the
// player's feet it's a tackle, loose it's a clearance, and either way the
// ball goes back up the pitch while the defender gets its breath.
func (m *Model) challenge() {
	bx, by := cell(m.ball.Pos.X), cell(m.ball.Pos.Y)
	px, py := cell(m.player.X), cell(m.player.Y)
	for i := range m.defs {
		d := &m.defs[i]
		if d.rest > 0 {
			continue
		}
		onBall := d.Covers(bx, by)
		onPlayer := m.dribbling && d.Covers(px, py)
		if !onBall && !onPlayer {
			continue
		}
		if m.dribbling {
			m.say(fmt.Sprintf("tackled by %q!", d.Name))
		} else {
			m.say(fmt.Sprintf("cleared by %q", d.Name))
		}
		m.dribbling = false
		m.trail = nil
		m.ball.Vel = Vec{-(3 + m.rng.Float64()*3), (m.rng.Float64()*2 - 1) * 2}
		d.rest = restTicks
		return
	}
}

func (m *Model) say(s string) { m.toast, m.toastLeft = s, toastTicks }

// kind is what a cell shows, which picks its style.
type kind int

const (
	kindText kind = iota
	kindChalk
	kindPlayer
	kindBall
	kindDefender
	// kindSpark is the first of hues firework colours.
	kindSpark
	kinds = kindSpark + hues
)

// glyph is a rune and the kind it's drawn as.
type glyph struct {
	r rune
	k kind
}

// palette is every cell style, per stripe of grass and for the flash the
// pitch gives on a goal.
func palette() [3][kinds]lipgloss.Style {
	grass := [3]lipgloss.Color{"#2F6B3A", "#367A41", "#5BA866"}
	sparks := [hues]lipgloss.Color{"#FFD23F", "#FF5DA2", "#5AD7FF", "#C8F25A"}
	var p [3][kinds]lipgloss.Style
	for i, g := range grass {
		base := lipgloss.NewStyle().Background(g)
		p[i][kindText] = base.Foreground(lipgloss.Color("#9DBE9F"))
		p[i][kindChalk] = base.Foreground(lipgloss.Color("#F4F1E6")).Bold(true)
		p[i][kindPlayer] = base.Foreground(lipgloss.Color("#C8F25A")).Bold(true)
		p[i][kindBall] = base.Foreground(lipgloss.Color("#FFFFFF"))
		p[i][kindDefender] = base.Foreground(lipgloss.Color("#FF6B5A")).Bold(true)
		for h, c := range sparks {
			p[i][kindSpark+kind(h)] = base.Foreground(c).Bold(true)
		}
	}
	return p
}

// View draws the pitch.
func (m Model) View() string {
	if m.tooSmall {
		return "pitch too small: make the window bigger (esc to go back)"
	}
	if m.over {
		best := max(m.best, m.goals)
		return lipgloss.Place(m.f.W, m.f.H, lipgloss.Center, lipgloss.Center, fmt.Sprintf(
			"FULL TIME · %d goals from %d shots · best %d\n\nenter play again · esc back", m.goals, m.shots, best))
	}
	help := "←↑↓→ run · space shoot · esc back"
	if m.party.on() {
		help = "space kick off · esc back"
	}
	bar := []rune(fmt.Sprintf(" ● kickabout   goals %d   shots %d   %d s   %s ", m.goals, m.shots, m.left/fps, help))
	var toast []rune
	switch {
	case m.party.on():
		toast = []rune(fmt.Sprintf(" %s  that's %d", m.party.chant(), m.party.goals))
	case m.toastLeft > 0:
		toast = []rune(" " + m.toast)
	}
	var over map[int]glyph
	if m.party.on() {
		over = m.party.overlay(m.f.W, m.f.H, m.plain)
	}
	defs := m.defenderCells()
	rows := make([]string, m.f.H)
	for y := range rows {
		var b strings.Builder
		runStripe, runKind := -1, kind(-1)
		var run strings.Builder
		flush := func() {
			if run.Len() == 0 {
				return
			}
			if m.plain {
				b.WriteString(run.String())
			} else {
				b.WriteString(m.paint[runStripe][runKind].Render(run.String()))
			}
			run.Reset()
		}
		for x := 0; x < m.f.W; x++ {
			g := m.cell(x, y, bar, toast, over, defs)
			s := (x / stripe) % 2
			if m.party.on() && m.party.flash() {
				s = 2
			}
			if s != runStripe || g.k != runKind {
				flush()
				runStripe, runKind = s, g.k
			}
			run.WriteRune(g.r)
		}
		flush()
		rows[y] = b.String()
	}
	return strings.Join(rows, "\n")
}

// defenderCells is every defender's runes, by y*W+x. During a
// celebration they hang their heads, faded like the scenery.
func (m Model) defenderCells() map[int]glyph {
	k := kindDefender
	if m.party.on() {
		k = kindText
	}
	out := map[int]glyph{}
	for _, d := range m.defs {
		x, y := cell(d.Pos.X), cell(d.Pos.Y)
		for i, r := range []rune(d.Name) {
			if m.f.inside(x+i, y) {
				out[y*m.f.W+x+i] = glyph{r, k}
			}
		}
	}
	return out
}

// cell is what (x, y) shows: the score bar and toast over the top rows,
// then any celebration, the player, the ball, the defenders, the goal and
// the ball's trail, and otherwise the screen's own text, faded.
func (m Model) cell(x, y int, bar, toast []rune, over, defs map[int]glyph) glyph {
	switch {
	case y == 0 && x < len(bar):
		return glyph{bar[x], kindChalk}
	case y == 1 && x < len(toast):
		return glyph{toast[x], kindChalk}
	}
	if g, ok := over[y*m.f.W+x]; ok {
		return g
	}
	inNet := m.party.on() && x == m.f.W-1 && m.goal.Open(y)
	switch {
	case inNet:
		if y == m.party.row {
			return glyph{m.ballRune(), kindBall}
		}
		return glyph{m.party.net(y), kindChalk}
	case m.party.on() && x == m.f.W-2 && m.goal.Open(y):
		return glyph{'╱', kindChalk}
	case x == cell(m.player.X) && y == cell(m.player.Y):
		if m.plain {
			return glyph{'@', kindPlayer}
		}
		return glyph{'◉', kindPlayer}
	case !m.party.on() && x == cell(m.ball.Pos.X) && y == cell(m.ball.Pos.Y):
		return glyph{m.ballRune(), kindBall}
	}
	if g, ok := defs[y*m.f.W+x]; ok {
		return g
	}
	switch {
	case x == m.f.W-1 && m.goal.Open(y):
		return glyph{'┃', kindChalk}
	case x == m.f.W-2 && m.goal.Open(y):
		return glyph{'╱', kindChalk}
	}
	for _, t := range m.trail {
		if cell(t.X) == x && cell(t.Y) == y {
			return glyph{'·', kindChalk}
		}
	}
	return glyph{m.f.Rune(x, y), kindText}
}

func (m Model) ballRune() rune {
	if m.plain {
		return 'o'
	}
	return '●'
}
