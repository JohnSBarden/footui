package kickabout

import (
	"math"
	"math/rand/v2"
	"strings"
)

// A goal stops play for a celebration: the pitch flashes, the net
// ripples, fireworks go up (the first out of the goal itself) and a
// block-letter GOAL! sweeps in, waves along its letters in the spark
// colours and sweeps out, while the toast chants a GOOOAL that keeps
// growing. Space cuts it short.

const (
	partyTicks = 3 * fps
	flashTicks = 6
	slideIn    = 15
	slideOut   = 12
	gravity    = 0.035
	sparkDrag  = 0.95
	hues       = 4
)

// bursts are the ticks a firework goes off on; the first is the net.
var bursts = []int{0, 9, 20, 33, 47, 60}

// spark is one fleck of a firework, fading over full ticks.
type spark struct {
	Pos, Vel   Vec
	life, full int
	hue        int
}

// party is a goal being celebrated, for left more ticks.
type party struct {
	left   int
	row    int // the row the ball went in on
	goals  int // the score it made
	sparks []spark
}

func (p party) on() bool { return p.left > 0 }

// t is how many ticks the celebration has run.
func (p party) t() int { return partyTicks - p.left }

// advance runs the celebration a tick: fireworks go up on their ticks and
// every spark flies on, falling and fading.
func (p *party) advance(f Field, rng *rand.Rand) {
	t := p.t()
	for i, at := range bursts {
		if at != t {
			continue
		}
		if i == 0 {
			p.sparks = append(p.sparks, burst(Vec{float64(f.W - 2), float64(p.row)}, 40, true, rng)...)
			continue
		}
		from := Vec{float64(f.W/6 + rng.IntN(max(1, f.W*2/3))), float64(3 + rng.IntN(max(1, f.H/2)))}
		p.sparks = append(p.sparks, burst(from, 26, false, rng)...)
	}
	live := p.sparks[:0]
	for _, s := range p.sparks {
		s.Pos.X += s.Vel.X
		s.Pos.Y += s.Vel.Y
		s.Vel.X *= sparkDrag
		s.Vel.Y = s.Vel.Y*sparkDrag + gravity
		s.life--
		if s.life > 0 && f.inside(cell(s.Pos.X), cell(s.Pos.Y)) {
			live = append(live, s)
		}
	}
	p.sparks = live
	p.left--
}

// burst is n sparks flying out of from in a ring, or, back from the net,
// a fan thrown back up the pitch.
func burst(from Vec, n int, back bool, rng *rand.Rand) []spark {
	ss := make([]spark, n)
	for i := range ss {
		a := 2*math.Pi*float64(i)/float64(n) + rng.Float64()*0.3
		v := 0.45 + rng.Float64()*0.6
		if back {
			a = math.Pi/2 + math.Pi*float64(i)/float64(n-1)
			v += 0.4
		}
		life := 18 + rng.IntN(16)
		ss[i] = spark{
			Pos:  from,
			Vel:  Vec{math.Cos(a) * v * 2, math.Sin(a) * v},
			life: life, full: life,
			hue: rng.IntN(hues),
		}
	}
	return ss
}

// glyph is a spark's look as it fades: a star, then a fleck, then a dot.
func (s spark) glyph(plain bool) rune {
	switch left := float64(s.life) / float64(s.full); {
	case left > 0.6 && plain:
		return '*'
	case left > 0.6:
		return '✦'
	case left > 0.3 && plain:
		return '+'
	case left > 0.3:
		return '*'
	case plain:
		return '.'
	}
	return '·'
}

// font is the banner's block letters, five rows each, every column drawn
// twice across so they keep their shape in tall cells.
var font = map[rune][5]string{
	'G': {" ### ", "#    ", "# ###", "#   #", " ### "},
	'O': {" ### ", "#   #", "#   #", "#   #", " ### "},
	'A': {" ### ", "#   #", "#####", "#   #", "#   #"},
	'L': {"#    ", "#    ", "#    ", "#    ", "#####"},
	'!': {"#", "#", "#", " ", "#"},
}

const letterGap = 2

// cheers are the banner's words, longest first; the widest that fits wins.
var cheers = []string{"GOOOOAL!", "GOOOAL!", "GOOAL!", "GOAL!"}

func letterWidth(r rune) int { return 2 * len(font[r][0]) }

func bannerWidth(word string) int {
	w := 0
	for _, r := range word {
		w += letterWidth(r) + letterGap
	}
	return w - letterGap
}

// cheer is the longest banner word that fits width with room either side.
func cheer(width int) string {
	for _, c := range cheers {
		if bannerWidth(c) <= width-4 {
			return c
		}
	}
	return cheers[len(cheers)-1]
}

// chant is the toast's GOOOAL, a few more O's every few ticks.
func (p party) chant() string {
	return "G" + strings.Repeat("O", 2+min(p.t()/3, 12)) + "AL!"
}

// overlay is what the celebration draws over a w×h pitch, by y*w+x: the
// sparks, with the banner on top.
func (p party) overlay(w, h int, plain bool) map[int]glyph {
	out := map[int]glyph{}
	put := func(x, y int, g glyph) {
		if x >= 0 && x < w && y >= 2 && y < h {
			out[y*w+x] = g
		}
	}
	for _, s := range p.sparks {
		put(cell(s.Pos.X), cell(s.Pos.Y), glyph{s.glyph(plain), kindSpark + kind(s.hue)})
	}

	t, word := p.t(), cheer(w)
	bw := bannerWidth(word)
	home := float64((w - bw) / 2)
	x := home
	switch leave := partyTicks - slideOut; {
	case t < slideIn:
		e := 1 - math.Pow(1-float64(t)/slideIn, 3)
		x = float64(w) + (home-float64(w))*e
	case t > leave:
		u := float64(t-leave) / slideOut
		x = home + (-float64(bw)-home)*u*u
	}
	top := (h - 5) / 2
	block := '█'
	if plain {
		block = '#'
	}
	lx := int(math.Round(x))
	// Clear a box behind the banner, room for the wave and a column of
	// grass either side, so the letters read over a busy screen.
	for y := top - 2; y < top+7; y++ {
		for c := lx - 2; c < lx+bw+2; c++ {
			put(c, y, glyph{' ', kindChalk})
		}
	}
	for i, r := range word {
		lift := 0
		if t >= slideIn {
			lift = int(math.Round(math.Sin(float64(t)*0.35 - float64(i)*0.9)))
		}
		k := kindSpark + kind((t/4+i)%hues)
		for row, line := range font[r] {
			for c, ch := range line {
				if ch == '#' {
					put(lx+2*c, top+row+lift, glyph{block, k})
					put(lx+2*c+1, top+row+lift, glyph{block, k})
				}
			}
		}
		lx += letterWidth(r) + letterGap
	}
	return out
}

// net is the goal mouth's post while the ball's in it, rippling.
func (p party) net(y int) rune {
	return []rune{'┃', '╲', '┃', '╱'}[(p.t()/2+y)%4]
}

// flash reports whether the pitch is lit this tick.
func (p party) flash() bool { return p.t() < flashTicks && p.t()%2 == 0 }
