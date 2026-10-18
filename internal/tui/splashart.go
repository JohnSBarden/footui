package tui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/JohnSBarden/footui/internal/tui/styles"
)

// The splash wordmark is "footui" in block letters, its two o's a pair of
// footballs bouncing out of step and spinning opposite ways. Each ball is
// a shaded sphere with the twelve pentagons of a truncated icosahedron
// picked out in the faint color, lit from the top left.

const (
	ballRows, ballCols = 7, 12
	ballTilt           = 0.38 // radians the ball leans towards the viewer
	ballLift           = 2.4  // rows a ball rises at the top of its bounce
	// artRows is the tallest letter plus the shadow row beneath the balls.
	artRows = 10
	// ballTop is the row a resting ball starts on, level with the u.
	ballTop = 2
	artGap  = 2
	// bounceRate is the bounce's angular speed; |sin| repeats every π,
	// so a ball lands about every 0.67 s.
	bounceRate = 2 * math.Pi * 0.75
	spinRate   = 3.2 // radians per second at full spin
)

var (
	glyphF = []string{"   ████ ", "  ██    ", "████████", "  ██    ", "  ██    ", "  ██    ", "  ██    ", "  ██    ", "  ██    "}
	glyphT = []string{"  ██    ", "  ██    ", "████████", "  ██    ", "  ██    ", "  ██    ", "  ██    ", "  ██    ", "   ████ "}
	glyphU = []string{"         ", "         ", "██     ██", "██     ██", "██     ██", "██     ██", "██     ██", "██     ██", " ███████ "}
	glyphI = []string{"██", "  ", "██", "██", "██", "██", "██", "██", "██"}
)

// artWidth is the wordmark's width in columns.
var artWidth = len([]rune(glyphF[0])) + 2*ballCols + len([]rune(glyphT[0])) +
	len([]rune(glyphU[0])) + len([]rune(glyphI[0])) + 5*artGap

// pentagons are the icosahedron's twelve vertices, which are where a
// truncated icosahedron (a football) has its pentagons.
var pentagons = func() [][3]float64 {
	phi := (1 + math.Sqrt(5)) / 2
	var v [][3]float64
	for _, a := range []float64{-1, 1} {
		for _, b := range []float64{-phi, phi} {
			v = append(v, unit(0, a, b), unit(a, b, 0), unit(b, 0, a))
		}
	}
	return v
}()

var ballLight = unit(-0.45, 0.55, 0.7)

func unit(x, y, z float64) [3]float64 {
	l := math.Sqrt(x*x + y*y + z*z)
	return [3]float64{x / l, y / l, z / l}
}

// ballCell is what one character of a ball shows.
type ballCell uint8

const (
	cellNone  ballCell = iota // outside the ball
	cellPanel                 // a pentagon
	cellLit                   // white panel, facing the light
	cellMid
	cellShade
)

// renderBall shades a ball spun spin radians about its vertical axis.
func renderBall(spin float64) [ballRows][ballCols]ballCell {
	var out [ballRows][ballCols]ballCell
	cy, sy := math.Cos(spin), math.Sin(spin)
	cx, sx := math.Cos(ballTilt), math.Sin(ballTilt)
	for r := 0; r < ballRows; r++ {
		for c := 0; c < ballCols; c++ {
			x := (float64(c)+0.5)/ballCols*2 - 1
			y := 1 - (float64(r)+0.5)/ballRows*2
			d := x*x + y*y
			if d > 1.04 {
				continue
			}
			z := math.Sqrt(math.Max(0, 1-d))
			// View space to ball space: undo the tilt, then the spin.
			y1, z1 := y*cx+z*sx, -y*sx+z*cx
			x2, z2 := x*cy-z1*sy, x*sy+z1*cy
			m := -1.0
			for _, v := range pentagons {
				m = math.Max(m, x2*v[0]+y1*v[1]+z2*v[2])
			}
			l := x*ballLight[0] + y*ballLight[1] + z*ballLight[2]
			switch {
			case m > 0.95:
				out[r][c] = cellPanel
			case l > 0.3:
				out[r][c] = cellLit
			case l > 0.05:
				out[r][c] = cellMid
			default:
				out[r][c] = cellShade
			}
		}
	}
	return out
}

// splashBalls is the two balls' motion. Each bounce's height is
// |sin(phase)|·amp; once the splash has nothing left to wait for, amp and
// spin run down so the balls come to rest.
type splashBalls struct {
	amp, spin float64
	s1, s2    float64 // spin angles
	p1, p2    float64 // bounce phases, a quarter turn apart
}

func newSplashBalls() splashBalls {
	return splashBalls{amp: 1, spin: 1, s2: 1.2, p2: math.Pi / 2}
}

// step advances the balls dt seconds; settling starts them coming to rest.
func (b *splashBalls) step(dt float64, settling bool) {
	if settling {
		b.amp = math.Max(0, b.amp-dt*1.4)
		b.spin = math.Max(0, b.spin-dt*1.1)
	}
	w := bounceRate * dt * (0.4 + 0.6*b.amp)
	b.p1 += w
	b.p2 += w
	b.s1 += spinRate * b.spin * dt
	b.s2 -= spinRate * b.spin * dt
}

// splashArt draws the wordmark, artRows lines of artWidth columns.
func splashArt(th styles.Theme, balls splashBalls) string {
	letter := th.TableCell
	h1 := math.Abs(math.Sin(balls.p1)) * balls.amp
	h2 := math.Abs(math.Sin(balls.p2)) * balls.amp
	b1, b2 := renderBall(balls.s1), renderBall(balls.s2)
	gap := strings.Repeat(" ", artGap)
	glyph := func(g []string, row int) string {
		if row >= len(g) {
			return strings.Repeat(" ", len([]rune(g[0])))
		}
		return letter.Render(g[row])
	}
	lines := make([]string, artRows)
	for row := range lines {
		lines[row] = strings.Join([]string{
			glyph(glyphF, row),
			ballRow(th, &b1, h1, row),
			ballRow(th, &b2, h2, row),
			glyph(glyphT, row),
			glyph(glyphU, row),
			glyph(glyphI, row),
		}, gap)
	}
	return strings.Join(lines, "\n")
}

// ballRow is one row of a ball's column: the ball itself, lifted h of
// the way to the top of its bounce, or on the last row its shadow, which
// shrinks as the ball rises.
func ballRow(th styles.Theme, ball *[ballRows][ballCols]ballCell, h float64, row int) string {
	if row == artRows-1 {
		w := max(4, int(math.Round(10-h*6)))
		left := (ballCols - w) / 2
		return strings.Repeat(" ", left) + th.Faint.Render(strings.Repeat("▀", w)) + strings.Repeat(" ", ballCols-left-w)
	}
	r := row - (ballTop - int(math.Round(h*ballLift)))
	if r < 0 || r >= ballRows {
		return strings.Repeat(" ", ballCols)
	}
	var b strings.Builder
	// Runs of one style are rendered together to keep the escapes down.
	var run strings.Builder
	var runStyle *lipgloss.Style
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if runStyle == nil {
			b.WriteString(run.String())
		} else {
			b.WriteString(runStyle.Render(run.String()))
		}
		run.Reset()
	}
	for _, cell := range ball[r] {
		ch, st := " ", (*lipgloss.Style)(nil)
		switch cell {
		case cellPanel:
			// Without color a pentagon can't be told from a white panel
			// by shade, so it is left as a hole.
			if !th.NoColor {
				ch, st = "█", &th.Faint
			}
		case cellLit:
			ch, st = "█", &th.TableCell
		case cellMid:
			ch, st = "▓", &th.TableCell
		case cellShade:
			ch, st = "▒", &th.TableCell
		}
		if st != runStyle {
			flush()
			runStyle = st
		}
		run.WriteString(ch)
	}
	flush()
	return b.String()
}
