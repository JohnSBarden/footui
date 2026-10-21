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

func TestBallRollsOverTextButBouncesOffABlock(t *testing.T) {
	f := field(
		"          ",
		"      ab  ",
		"          ",
	)
	b := Ball{Pos: Vec{3, 1}, Vel: Vec{2, 0}}
	b, hit, _ := Step(b, f, nil, nil)
	if b.Vel.X <= 0 || hit != nil {
		t.Errorf("text is scenery: vel %+v hit %v", b.Vel, hit)
	}

	block := func(x, y int) bool { return x == 6 && y == 1 }
	b = Ball{Pos: Vec{3, 1}, Vel: Vec{2, 0}}
	for i := 0; i < 3 && hit == nil; i++ {
		b, hit, _ = Step(b, f, block, nil)
	}
	if b.Vel.X >= 0 {
		t.Errorf("ball should bounce back off the block, vel %+v", b.Vel)
	}
	if hit == nil || *hit != [2]int{6, 1} {
		t.Errorf("hit %v should be the blocked cell", hit)
	}
}

func TestBallBouncesOffTheEdgesWithoutNamingAWord(t *testing.T) {
	f := blank(10, 5)
	b := Ball{Pos: Vec{1, 2}, Vel: Vec{-2, 0}}
	b, hit, _ := Step(b, f, nil, nil)
	if b.Vel.X <= 0 || hit != nil {
		t.Errorf("left edge: vel %+v hit %v", b.Vel, hit)
	}
	b = Ball{Pos: Vec{5, 1}, Vel: Vec{0, -2}}
	b, _, _ = Step(b, f, nil, nil)
	if b.Vel.Y <= 0 {
		t.Errorf("top edge: vel %+v", b.Vel)
	}
}

func TestFrictionStopsTheBall(t *testing.T) {
	f := blank(40, 10)
	b := Ball{Pos: Vec{2, 5}, Vel: Vec{3, 1}}
	for i := 0; i < 300; i++ {
		b, _, _ = Step(b, f, nil, nil)
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
	if _, _, scored := Step(in, f, nil, g.Open); !scored {
		t.Error("a ball through the mouth scores")
	}
	wide := Ball{Pos: Vec{17, 1}, Vel: Vec{4, 0}}
	b, _, scored := Step(wide, f, nil, g.Open)
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

func TestPlayerRunsOverTextButNotThroughABlock(t *testing.T) {
	f := field("  x   ")
	if p := MovePlayer(Vec{1, 0}, Vec{2, 0}, f, nil); p != (Vec{3, 0}) {
		t.Errorf("text is scenery: %+v", p)
	}
	block := func(x, y int) bool { return x == 2 }
	if p := MovePlayer(Vec{1, 0}, Vec{2, 0}, f, block); p != (Vec{1, 0}) {
		t.Errorf("walked into the block: %+v", p)
	}
	if p := MovePlayer(Vec{4, 0}, Vec{2, 0}, f, nil); p != (Vec{5, 0}) {
		t.Errorf("should stop at the edge: %+v", p)
	}
}

func TestWordsSkipsRulesAndNumbers(t *testing.T) {
	f := field("  Chelsea 90 ──── Man Utd x A1  ")
	var got []string
	for _, w := range f.Words() {
		got = append(got, w.Text)
	}
	if strings.Join(got, ",") != "Chelsea,Man,Utd,A1" {
		t.Errorf("Words = %q", got)
	}
	f.Lift(f.Words()[0])
	if f.Rune(2, 0) != ' ' || f.Rune(8, 0) != ' ' {
		t.Error("Lift should blank the word")
	}
}
