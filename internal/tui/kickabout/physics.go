package kickabout

import "math"

// Vec is a position or velocity in cells. Terminal cells are about twice
// as tall as wide, so horizontal speeds are doubled where they're set.
type Vec struct{ X, Y float64 }

type Ball struct{ Pos, Vel Vec }

const (
	friction  = 0.92
	stopBelow = 0.05
	// substeps split a tick's movement so a fast ball can't jump clean
	// over a one-cell word.
	substeps = 4
)

// Step advances the ball one tick, bouncing off edges and blocked cells
// except where mouth opens the right edge (a goal). hit is the blocked
// cell bounced off, nil for an edge or no bounce.
func Step(b Ball, f Field, block func(x, y int) bool, mouth func(y int) bool) (next Ball, hit *[2]int, scored bool) {
	solid := func(x, y int) bool { return f.blocked(x, y, block) }
	for i := 0; i < substeps; i++ {
		nx := b.Pos.X + b.Vel.X/substeps
		cx, cy := cell(nx), cell(b.Pos.Y)
		switch {
		case cx >= f.W && mouth != nil && mouth(cy):
			b.Pos.X = nx
			return b, hit, true
		case solid(cx, cy):
			if f.inside(cx, cy) {
				hit = &[2]int{cx, cy}
			}
			b.Vel.X = -b.Vel.X
		default:
			b.Pos.X = nx
		}

		ny := b.Pos.Y + b.Vel.Y/substeps
		cx, cy = cell(b.Pos.X), cell(ny)
		if solid(cx, cy) {
			if f.inside(cx, cy) {
				hit = &[2]int{cx, cy}
			}
			b.Vel.Y = -b.Vel.Y
		} else {
			b.Pos.Y = ny
		}
	}
	b.Vel.X *= friction
	b.Vel.Y *= friction
	if math.Hypot(b.Vel.X, b.Vel.Y) < stopBelow {
		b.Vel = Vec{}
	}
	return b, hit, false
}

func cell(v float64) int { return int(math.Floor(v)) }

// MovePlayer moves p by d a cell at a time, stopping short of blocks.
func MovePlayer(p, d Vec, f Field, block func(x, y int) bool) Vec {
	steps := int(math.Max(math.Abs(d.X), math.Abs(d.Y)))
	if steps == 0 {
		return p
	}
	step := Vec{d.X / float64(steps), d.Y / float64(steps)}
	for i := 0; i < steps; i++ {
		n := Vec{p.X + step.X, p.Y + step.Y}
		if f.blocked(cell(n.X), cell(n.Y), block) {
			break
		}
		p = n
	}
	return p
}

// goalEvery is how many ticks the goal waits between moving a row.
const goalEvery = 4

// Goal is the mouth on the right edge, Size rows from Top.
type Goal struct {
	Top, Size, Dir int
	tick           int
}

// Open reports whether row y is inside the mouth.
func (g Goal) Open(y int) bool { return y >= g.Top && y < g.Top+g.Size }

// Advance keeps the goal within rows 1 to height-2.
func (g Goal) Advance(height int) Goal {
	g.tick++
	if g.tick%goalEvery != 0 {
		return g
	}
	if g.Dir == 0 {
		g.Dir = 1
	}
	next := g.Top + g.Dir
	if next < 1 || next+g.Size > height-1 {
		g.Dir = -g.Dir
		next = g.Top + g.Dir
	}
	if next >= 1 && next+g.Size <= height-1 {
		g.Top = next
	}
	return g
}
