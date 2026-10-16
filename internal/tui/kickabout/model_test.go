package kickabout

import (
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

func TestShootingThroughTheMouthScores(t *testing.T) {
	m := New("", 80, 24, styles.Plain())
	m.player = Vec{70, 10}
	m.facing = Vec{1, 0}
	m.ball = Ball{Pos: Vec{71, 10}}
	m.goal = Goal{Top: 8, Size: 5, Dir: 1}
	m, _ = m.Update(space)
	for i := 0; i < 10 && m.goals == 0; i++ {
		m, _ = m.Update(TickMsg{gen: m.gen})
	}
	if m.shots != 1 || m.goals != 1 {
		t.Errorf("shots %d goals %d", m.shots, m.goals)
	}
	if !m.dribbling {
		t.Error("after a goal the ball comes back to the player")
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
