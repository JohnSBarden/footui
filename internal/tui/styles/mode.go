package styles

import (
	"fmt"
	"strings"
)

// Mode is how strongly a club's colours are applied to its screens.
type Mode int

const (
	// Weak keeps the terminal's background; club colours are highlights only.
	Weak Mode = iota
	// Strong paints the whole screen in the club's primary colour.
	Strong
)

func (m Mode) String() string {
	if m == Strong {
		return "strong"
	}
	return "weak"
}

// ParseMode reads a mode name, as given to --theme or FOOTUI_THEME.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "weak":
		return Weak, nil
	case "strong":
		return Strong, nil
	}
	return Weak, fmt.Errorf("unknown theme %q (want weak or strong)", s)
}

// Appearance is the user's theme choice. It is shared by pointer so a
// runtime toggle reaches screens built from older copies of their deps.
type Appearance struct {
	Mode Mode
}

// Toggle flips between weak and strong.
func (a *Appearance) Toggle() {
	if a.Mode == Strong {
		a.Mode = Weak
	} else {
		a.Mode = Strong
	}
}
