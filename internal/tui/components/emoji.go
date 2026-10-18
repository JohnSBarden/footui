package components

import (
	"os"
	"strings"
)

// Emoji reports whether the terminal can draw colour emoji. The Linux
// console and dumb or VT-style terminals can't, so screens fall back to
// plain glyphs there. A variable so tests can pin it.
var Emoji = emojiTerminal(os.Getenv("TERM"))

func emojiTerminal(term string) bool {
	return term != "linux" && term != "dumb" && !strings.HasPrefix(term, "vt")
}
