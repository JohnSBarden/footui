package components

import (
	"os"
	"strings"
)

// Emoji reports whether the terminal can draw colour emoji (not the Linux
// console or dumb/VT terminals). A variable so tests can pin it.
var Emoji = emojiTerminal(os.Getenv("TERM"))

func emojiTerminal(term string) bool {
	return term != "linux" && term != "dumb" && !strings.HasPrefix(term, "vt")
}
