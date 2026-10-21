package kickabout

import (
	"math"
	"math/rand/v2"
)

const (
	minDefenders, maxDefenders = 3, 5
	// defSpeed is a defender's pace in rows a tick, doubled across like
	// the player's. A held key repeats about fps times a second, a row a
	// press, so this is a third of the player's pace.
	defSpeed = 1.0 / 3
	// restTicks is how long a defender stands after winning the ball,
	// which is also how long the ball can roll out of it.
	restTicks = fps
	// shuffle is how far towards the ball a defender not pressing it
	// strays from home.
	shuffle = 0.35
)

// standIns are defenders for a screen without enough words to pick from.
var standIns = []string{"Maldini", "Baresi", "Puyol", "Cannavaro", "Kanté", "Ramos", "Terry", "Rodri"}

// Defender is a word from the screen come alive: it covers its runes from
// Pos along one row.
type Defender struct {
	Name      string
	Pos, Home Vec
	// rest counts down the ticks it stands still after a tackle.
	rest int
}

func (d Defender) width() int { return len([]rune(d.Name)) }

// centre is the middle of the word.
func (d Defender) centre() Vec { return Vec{d.Pos.X + float64(d.width())/2, d.Pos.Y} }

// Covers reports whether the word is on (x, y).
func (d Defender) Covers(x, y int) bool {
	left := cell(d.Pos.X)
	return y == cell(d.Pos.Y) && x >= left && x < left+d.width()
}

// near reports whether d's word comes within gap columns of o's on the
// same or a neighbouring row.
func (d Defender) near(o Defender, gap int) bool {
	dy := cell(d.Pos.Y) - cell(o.Pos.Y)
	if dy < -1 || dy > 1 {
		return false
	}
	a, b := cell(d.Pos.X), cell(o.Pos.X)
	return a < b+o.width()+gap && b < a+d.width()+gap
}

// pickDefenders lifts a few of the field's words off it to defend, keeping
// them clear of the kick-off spot, the goal, the score bar and each other,
// and makes up the numbers with stand-ins when the screen is too bare.
func pickDefenders(f Field, kickOff Vec, rng *rand.Rand) []Defender {
	n := minDefenders + rng.IntN(maxDefenders-minDefenders+1)
	var ds []Defender
	fits := func(d Defender) bool {
		x, y, w := cell(d.Pos.X), cell(d.Pos.Y), d.width()
		if y < 2 || y > f.H-2 || x < cell(kickOff.X)+12 || x+w > f.W-4 {
			return false
		}
		for _, o := range ds {
			if d.near(o, 4) {
				return false
			}
		}
		return true
	}
	words := f.Words()
	rng.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })
	for _, w := range words {
		if len(ds) == n {
			break
		}
		at := Vec{float64(w.X), float64(w.Y)}
		if d := (Defender{Name: w.Text, Pos: at, Home: at}); fits(d) {
			f.Lift(w)
			ds = append(ds, d)
		}
	}
	names := rng.Perm(len(standIns))
	for tries := 0; len(ds) < n && tries < 200; tries++ {
		name := standIns[names[len(ds)%len(names)]]
		at := Vec{float64(f.W/3 + rng.IntN(max(1, f.W/2))), float64(2 + rng.IntN(max(1, f.H-3)))}
		if d := (Defender{Name: name, Pos: at, Home: at}); fits(d) {
			ds = append(ds, d)
		}
	}
	return ds
}

// moveDefenders runs each defender a tick: whoever is nearest the ball
// presses it, and the rest shuffle across from home towards it, never
// onto one another or into the score bar or the goal.
func moveDefenders(ds []Defender, ball Vec, f Field) {
	press, best := -1, math.Inf(1)
	for i, d := range ds {
		if d.rest > 0 {
			continue
		}
		if dist := apart(d.centre(), ball); dist < best {
			press, best = i, dist
		}
	}
	for i := range ds {
		d := &ds[i]
		if d.rest > 0 {
			d.rest--
			continue
		}
		target := Vec{d.Home.X + float64(d.width())/2, d.Home.Y}
		if i == press {
			target = ball
		} else {
			target = Vec{target.X + (ball.X-target.X)*shuffle, target.Y + (ball.Y-target.Y)*shuffle}
		}
		next := toward(*d, target)
		next.X = math.Max(0, math.Min(next.X, float64(f.W-d.width()-3)))
		next.Y = math.Max(2, math.Min(next.Y, float64(f.H-1)))
		moved := *d
		moved.Pos = next
		clash := false
		for j, o := range ds {
			// One already alongside another may still step away.
			if j != i && moved.near(o, 1) && !d.near(o, 1) {
				clash = true
				break
			}
		}
		if !clash {
			d.Pos = next
		}
	}
}

// toward is where d's word gets to this tick running its centre at
// target, as far as defSpeed lets it.
func toward(d Defender, target Vec) Vec {
	c := d.centre()
	dist := apart(c, target)
	if dist < 0.01 {
		return d.Pos
	}
	step := math.Min(1, defSpeed/dist)
	return Vec{d.Pos.X + (target.X-c.X)*step, d.Pos.Y + (target.Y-c.Y)*step}
}

// apart is the distance from a to b in rows, counting two columns as one.
func apart(a, b Vec) float64 { return math.Hypot((b.X-a.X)/2, b.Y-a.Y) }
