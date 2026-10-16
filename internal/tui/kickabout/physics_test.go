package kickabout

import (
	"strings"
	"testing"
)

func field(rows ...string) Field {
	return NewField(strings.Join(rows, "\n"), len([]rune(rows[0])), len(rows))
}

func blank(w, h int) Field {
	rows := make([]string, h)
	for i := range rows {
		rows[i] = strings.Repeat(" ", w)
	}
	return field(rows...)
}

func TestBallBouncesOffAWord(t *testing.T) {
	f := field(
		"          ",
		"      ab  ",
		"          ",
	)
	b := Ball{Pos: Vec{3, 1}, Vel: Vec{2, 0}}
	var hit *[2]int
	for i := 0; i < 3 && hit == nil; i++ {
		b, hit, _ = Step(b, f, nil)
	}
	if b.Vel.X >= 0 {
		t.Errorf("ball should bounce back off 'ab', vel %+v", b.Vel)
	}
	if hit == nil || f.WordAt(hit[0], hit[1]) != "ab" {
		t.Errorf("hit %v should name the word", hit)
	}
}

func TestBallBouncesOffTheEdgesWithoutNamingAWord(t *testing.T) {
	f := blank(10, 5)
	b := Ball{Pos: Vec{1, 2}, Vel: Vec{-2, 0}}
	b, hit, _ := Step(b, f, nil)
	if b.Vel.X <= 0 || hit != nil {
		t.Errorf("left edge: vel %+v hit %v", b.Vel, hit)
	}
	b = Ball{Pos: Vec{5, 1}, Vel: Vec{0, -2}}
	b, _, _ = Step(b, f, nil)
	if b.Vel.Y <= 0 {
		t.Errorf("top edge: vel %+v", b.Vel)
	}
}

func TestFrictionStopsTheBall(t *testing.T) {
	f := blank(40, 10)
	b := Ball{Pos: Vec{2, 5}, Vel: Vec{3, 1}}
	for i := 0; i < 300; i++ {
		b, _, _ = Step(b, f, nil)
	}
	if b.Vel != (Vec{}) {
		t.Errorf("ball still moving: %+v", b.Vel)
	}
	if b.Pos.X < 0 || b.Pos.X >= 40 || b.Pos.Y < 0 || b.Pos.Y >= 10 {
		t.Errorf("ball left the field: %+v", b.Pos)
	}
}

func TestTheBallScoresOnlyThroughTheMouth(t *testing.T) {
	f := blank(20, 10)
	g := Goal{Top: 4, Size: 3}
	in := Ball{Pos: Vec{17, 5}, Vel: Vec{4, 0}}
	if _, _, scored := Step(in, f, g.Open); !scored {
		t.Error("a ball through the mouth scores")
	}
	wide := Ball{Pos: Vec{17, 1}, Vel: Vec{4, 0}}
	b, _, scored := Step(wide, f, g.Open)
	if scored || b.Vel.X >= 0 {
		t.Errorf("a ball wide of the mouth bounces back: scored %v vel %+v", scored, b.Vel)
	}
}

func TestGoalDriftsAndTurnsAtTheEdges(t *testing.T) {
	g := Goal{Top: 1, Size: 5, Dir: 1}
	seen := map[int]bool{}
	for i := 0; i < 400; i++ {
		g = g.Advance(12)
		seen[g.Top] = true
		if g.Top < 1 || g.Top+g.Size > 11 {
			t.Fatalf("goal left the edge: top %d", g.Top)
		}
	}
	if !seen[1] || !seen[6] {
		t.Errorf("goal should sweep the whole edge: %v", seen)
	}
}

func TestPlayerCannotWalkThroughText(t *testing.T) {
	f := field("  x  ")
	if p := MovePlayer(Vec{1, 0}, Vec{2, 0}, f); p != (Vec{1, 0}) {
		t.Errorf("walked into 'x': %+v", p)
	}
	if p := MovePlayer(Vec{3, 0}, Vec{1, 0}, f); p != (Vec{4, 0}) {
		t.Errorf("free cell: %+v", p)
	}
}

func TestWordAtNamesTheWholeRun(t *testing.T) {
	f := field("  Chelsea  ")
	if w := f.WordAt(4, 0); w != "Chelsea" {
		t.Errorf("WordAt = %q", w)
	}
	if w := f.WordAt(0, 0); w != "" {
		t.Errorf("space = %q", w)
	}
}
