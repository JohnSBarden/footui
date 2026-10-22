// Package textfold normalizes names for searching. SQLite's LIKE has no
// accent equivalence, so a folded copy of each name is stored and searched.
package textfold

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// standalone spells out letters NFD can't split (Ø, ß, Ł, ı, …), so
// "odegaard" finds Ødegaard.
var standalone = strings.NewReplacer(
	"Ø", "O", "ø", "o", "Æ", "Ae", "æ", "ae", "Œ", "Oe", "œ", "oe",
	"ß", "ss", "Ł", "L", "ł", "l", "Đ", "D", "đ", "d", "Ð", "D", "ð", "d",
	"Þ", "Th", "þ", "th", "ı", "i", "Ħ", "H", "ħ", "h",
)

// Fold returns a lowercase, accent-stripped form of s for search
// comparisons only; it is never displayed.
func Fold(s string) string {
	decomposed := norm.NFD.String(standalone.Replace(s))

	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		// Mn: the combining accents NFD split out.
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}

	return norm.NFC.String(b.String())
}
