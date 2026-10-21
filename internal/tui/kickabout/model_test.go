package kickabout

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

var space = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}

func TestTooSmallSaysSoAndEscEnds(t *testing.T) {
	m := New("", 40, 10, styles.Default())
	if !strings.Contains(m.View(), "too small") {
		t.Error("small terminal message missing")
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.Done() {
		t.Error("esc should end the game")
	}
}

func TestThePitchFillsTheScreenAndKeepsItsText(t *testing.T) {
	frame := "Premier League\n\n  1 Arsenal   90"
	m := New(frame, 80, 24, styles.Plain())
	lines := strings.Split(m.View(), "\n")
	if len(lines) != 24 {
		t.Fatalf("lines = %d", len(lines))
	}
	for i, l := range lines {
		if lipgloss.Width(l) != 80 {
			t.Fatalf("line %d width %d", i, lipgloss.Width(l))
		}
	}
	if !strings.Contains(m.View(), "Arsenal") || !strings.Contains(m.View(), "@") || !strings.Contains(m.View(), "o") {
		t.Errorf("pitch should keep the text and show the player and ball\n%s", m.View())
	}
}

// scoreOne shoots m's ball into the goal from close in, no defenders about.
func scoreOne(t *testing.T, m Model) Model {
	t.Helper()
	m.defs = nil
	m.player = Vec{70, 10}
	m.facing = Vec{1, 0}
	m.ball = Ball{Pos: Vec{71, 10}}
	m.goal = Goal{Top: 8, Size: 5, Dir: 1}
	m, _ = m.Update(space)
	for i := 0; i < 10 && m.goals == 0; i++ {
		m, _ = m.Update(TickMsg{gen: m.gen})
	}
	if m.shots != 1 || m.goals != 1 {
		t.Fatalf("shots %d goals %d", m.shots, m.goals)
	}
	return m
}

func TestAGoalIsCelebratedWithTheClockStopped(t *testing.T) {
	m := scoreOne(t, New("", 80, 24, styles.Plain()))
	if !m.party.on() || m.party.row < 8 || m.party.row > 12 {
		t.Fatalf("a goal should start a celebration at the net: %+v", m.party)
	}
	clock := m.left
	for range 20 {
		m, _ = m.Update(TickMsg{gen: m.gen})
	}
	if m.left != clock {
		t.Error("the clock should stop for the celebration")
	}
	if len(m.party.sparks) == 0 {
		t.Error("fireworks should be up")
	}
	v := m.View()
	if !strings.Contains(v, "#####") || !strings.Contains(v, "GOOOO") {
		t.Errorf("the banner and the chant should show\n%s", v)
	}
	before := m.player
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.player != before {
		t.Error("nobody runs during the celebration")
	}
	for i := 0; i < partyTicks && m.party.on(); i++ {
		m, _ = m.Update(TickMsg{gen: m.gen})
	}
	if m.party.on() || !m.dribbling || m.player != m.kickOffSpot() {
		t.Errorf("after the celebration it's kick-off: party %v dribbling %v player %+v", m.party.on(), m.dribbling, m.player)
	}
}

func TestSpaceCutsTheCelebrationShort(t *testing.T) {
	m := scoreOne(t, New("", 80, 24, styles.Plain()))
	m, _ = m.Update(space)
	if m.party.on() || !m.dribbling || m.shots != 1 {
		t.Errorf("space should go straight to kick-off without a shot: party %v shots %d", m.party.on(), m.shots)
	}
}

func TestTheCelebrationDrawsInColourAndStaysInBounds(t *testing.T) {
	m := scoreOne(t, New("", 80, 24, styles.Default()))
	for range 3 * fps {
		lines := strings.Split(m.View(), "\n")
		if len(lines) != 24 {
			t.Fatalf("lines = %d", len(lines))
		}
		for i, l := range lines {
			if lipgloss.Width(l) != 80 {
				t.Fatalf("tick %d line %d width %d", m.party.t(), i, lipgloss.Width(l))
			}
		}
		m, _ = m.Update(TickMsg{gen: m.gen})
	}
}

func TestAFewWordsComeAliveAsDefenders(t *testing.T) {
	frame := strings.Join([]string{
		"",
		"",
		"  1  Arsenal          Chelsea         Liverpool",
		"",
		"  2  Everton          Fulham          Brentford",
		"",
		"  3  Brighton         Burnley         Wolves",
		"",
		"  4  Tottenham        Newcastle       Bournemouth",
	}, "\n")
	m := New(frame, 80, 24, styles.Plain())
	if n := len(m.defs); n < minDefenders || n > maxDefenders {
		t.Fatalf("%d defenders", n)
	}
	for _, d := range m.defs {
		if !strings.Contains(frame, d.Name) {
			t.Errorf("defender %q should be a word from the screen", d.Name)
		}
		if m.f.Rune(cell(d.Home.X), cell(d.Home.Y)) != ' ' {
			t.Errorf("%q should have left its spot on the screen", d.Name)
		}
	}
}

func TestABareScreenStillGetsDefenders(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	if n := len(m.defs); n < minDefenders || n > maxDefenders {
		t.Errorf("%d defenders", n)
	}
}

func TestDefendersRunAtAThirdOfThePlayersPace(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	m.defs = []Defender{{Name: "Rodri", Pos: Vec{40, 18}, Home: Vec{40, 18}}}
	m.ball = Ball{Pos: Vec{42.5, 4}}
	m.dribbling = false
	m, _ = m.Update(TickMsg{gen: m.gen})
	if got := 18 - m.defs[0].Pos.Y; math.Abs(got-defSpeed) > 1e-9 {
		t.Errorf("ran %v rows in a tick, want %v", got, defSpeed)
	}
}

func TestADefenderTacklesThePlayer(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	m.player, m.facing = Vec{30, 10}, Vec{1, 0}
	m.ball = Ball{Pos: m.feet()}
	m.defs = []Defender{{Name: "Puyol", Pos: Vec{29, 10}, Home: Vec{50, 10}}}
	m, _ = m.Update(TickMsg{gen: m.gen})
	if m.dribbling || m.ball.Vel.X >= 0 {
		t.Errorf("the ball should be cleared back up the pitch: dribbling %v vel %+v", m.dribbling, m.ball.Vel)
	}
	if m.defs[0].rest == 0 || !strings.Contains(m.toast, "Puyol") {
		t.Errorf("rest %d toast %q", m.defs[0].rest, m.toast)
	}
	before := m.defs[0].Pos
	m, _ = m.Update(TickMsg{gen: m.gen})
	if m.defs[0].Pos != before {
		t.Error("a defender stands for a moment after a tackle")
	}
}

func TestThePlayerHasToGoRoundADefender(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	m.player, m.facing = Vec{30, 10}, Vec{1, 0}
	m.defs = []Defender{{Name: "Ramos", Pos: Vec{32, 10}, Home: Vec{32, 10}, rest: 5}}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.player.X != 31 {
		t.Errorf("player ran into the defender: %+v", m.player)
	}
}

func TestTheRoundEndsAndEnterPlaysAgain(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	m.goals = 3
	for i := 0; i < roundTicks+1; i++ {
		m, _ = m.Update(TickMsg{gen: m.gen})
	}
	if !strings.Contains(m.View(), "FULL TIME") {
		t.Fatal("round should end at 60 s")
	}
	old := m.gen
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || m.gen == old || m.goals != 0 || m.best != 3 {
		t.Errorf("enter should restart, keeping the best: gen %d goals %d best %d", m.gen, m.goals, m.best)
	}
	// A tick from the finished round is ignored.
	before := m.left
	m, _ = m.Update(TickMsg{gen: old})
	if m.left != before {
		t.Error("a stale tick advanced the new round")
	}
}

func TestPlayerMovesAndCarriesTheBall(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	start := m.player
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.player.Y != start.Y+1 || m.ball.Pos != (Vec{m.player.X, m.player.Y + 1}) {
		t.Errorf("player %+v ball %+v", m.player, m.ball.Pos)
	}
}
