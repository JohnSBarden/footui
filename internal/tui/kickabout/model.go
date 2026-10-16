package kickabout

import (
	"fmt"
	"math"
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
	paint  [2][kinds]lipgloss.Style
	f      Field
	player Vec
	facing Vec
	ball   Ball
	// dribbling is set while the ball is at the player's feet.
	dribbling bool
	goal      Goal
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
	m.f = NewField(frame, width, height)
	m.paint = palette()
	m.reset()
	return m
}

func (m *Model) reset() {
	m.player = m.freeCell(Vec{2, float64(m.f.H / 2)})
	m.facing = Vec{1, 0}
	m.ball = Ball{Pos: m.feet()}
	m.dribbling = true
	m.goal = Goal{Top: m.f.H/2 - goalSize/2, Size: goalSize, Dir: 1}
	m.goals, m.shots, m.left, m.over = 0, 0, roundTicks, false
	m.trail, m.toast, m.toastLeft = nil, "", 0
}

// feet is where the ball sits while the player dribbles it.
func (m Model) feet() Vec { return Vec{m.player.X + m.facing.X, m.player.Y + m.facing.Y} }

// freeCell searches from p for an empty cell with an empty one to its
// right, so the player never starts inside a word or without the ball.
func (m Model) freeCell(p Vec) Vec {
	for x := int(p.X); x < m.f.W-2; x++ {
		for dy := 0; dy < m.f.H-2; dy++ {
			y := 1 + (int(p.Y)-1+dy)%(m.f.H-2)
			if !m.f.Solid(x, y) && !m.f.Solid(x+1, y) {
				return Vec{float64(x), float64(y)}
			}
		}
	}
	return Vec{1, 1}
}

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
		if m.dribbling && !m.over && !m.tooSmall {
			m.dribbling = false
			m.shots++
			m.ball.Vel = Vec{m.facing.X * shotSpeed * 2, m.facing.Y * shotSpeed}
		}
		return m, nil
	}
	d, ok := moves[msg.String()]
	if !ok || m.over || m.tooSmall {
		return m, nil
	}
	m.facing = Vec{sign(d.X), sign(d.Y)}
	m.player = MovePlayer(m.player, d, m.f)
	if m.dribbling {
		if feet := m.feet(); !m.f.Solid(cell(feet.X), cell(feet.Y)) {
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
	m.left--
	if m.left <= 0 {
		m.over = true
		return
	}
	m.goal = m.goal.Advance(m.f.H)
	if m.toastLeft > 0 {
		m.toastLeft--
	}
	if m.dribbling {
		return
	}
	m.trail = append(m.trail, m.ball.Pos)
	if len(m.trail) > trailLen {
		m.trail = m.trail[1:]
	}
	next, hit, scored := Step(m.ball, m.f, m.goal.Open)
	if scored {
		m.goals++
		m.say("GOAL!")
		m.dribbling = true
		m.ball = Ball{Pos: m.feet()}
		m.trail = nil
		return
	}
	if hit != nil {
		if w := m.f.WordAt(hit[0], hit[1]); w != "" {
			m.say(fmt.Sprintf("bounced off %q", w))
		}
	}
	m.ball = next
	m.pickUp()
}

func (m *Model) say(s string) { m.toast, m.toastLeft = s, toastTicks }

// kind is what a cell shows, which picks its style.
type kind int

const (
	kindText kind = iota
	kindChalk
	kindPlayer
	kindBall
	kinds
)

// palette is every cell style, per stripe of grass.
func palette() [2][kinds]lipgloss.Style {
	grass := [2]lipgloss.Color{"#2F6B3A", "#367A41"}
	var p [2][kinds]lipgloss.Style
	for i, g := range grass {
		base := lipgloss.NewStyle().Background(g)
		p[i] = [kinds]lipgloss.Style{
			kindText:   base.Foreground(lipgloss.Color("#9DBE9F")),
			kindChalk:  base.Foreground(lipgloss.Color("#F4F1E6")).Bold(true),
			kindPlayer: base.Foreground(lipgloss.Color("#E0CCFF")).Bold(true),
			kindBall:   base.Foreground(lipgloss.Color("#FFFFFF")),
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
	bar := []rune(fmt.Sprintf(" ● kickabout   goals %d   shots %d   %d s   ←↑↓→ run · space shoot · esc back ", m.goals, m.shots, m.left/fps))
	var toast []rune
	if m.toastLeft > 0 {
		toast = []rune(" " + m.toast)
	}
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
			r, k := m.cell(x, y, bar, toast)
			s := (x / stripe) % 2
			if s != runStripe || k != runKind {
				flush()
				runStripe, runKind = s, k
			}
			run.WriteRune(r)
		}
		flush()
		rows[y] = b.String()
	}
	return strings.Join(rows, "\n")
}

// cell is what (x, y) shows: the score bar and toast over the top rows,
// then the player, the ball, the goal and the ball's trail, and otherwise
// the screen's own text, faded.
func (m Model) cell(x, y int, bar, toast []rune) (rune, kind) {
	switch {
	case y == 0 && x < len(bar):
		return bar[x], kindChalk
	case y == 1 && x < len(toast):
		return toast[x], kindChalk
	case x == cell(m.player.X) && y == cell(m.player.Y):
		if m.plain {
			return '@', kindPlayer
		}
		return '◉', kindPlayer
	case x == cell(m.ball.Pos.X) && y == cell(m.ball.Pos.Y):
		if m.plain {
			return 'o', kindBall
		}
		return '●', kindBall
	case x == m.f.W-1 && m.goal.Open(y):
		return '┃', kindChalk
	case x == m.f.W-2 && m.goal.Open(y):
		return '╱', kindChalk
	}
	for _, t := range m.trail {
		if cell(t.X) == x && cell(t.Y) == y {
			return '·', kindChalk
		}
	}
	return m.f.Rune(x, y), kindText
}
