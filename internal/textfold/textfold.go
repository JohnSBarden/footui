// Package textfold normalizes names for searching.
//
// Football is full of accented names — Martín Zubimendi, Jurriën Timber,
// Bayern München, Atlético Madrid — and people search for them from an
// ASCII keyboard. SQLite's LIKE is case-insensitive only for ASCII and has
// no notion of accent equivalence at all, so "martin" finds "Martin
// Odegaard" but not "Martín Zubimendi", and "jurrien" finds nothing.
//
// Folding a copy of each name at write time, and searching that, makes
// both work without asking the user to type diacritics they may not have
// keys for.
package textfold

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Fold returns a lowercase, accent-stripped form of s for search
// comparisons. It is never displayed — the original name is always what
// the user sees.
//
// The approach is canonical decomposition (NFD), which splits "í" into
// "i" plus a combining acute accent, followed by dropping the combining
// marks. That handles the whole Latin range without a hand-maintained
// character table.
func Fold(s string) string {
	decomposed := norm.NFD.String(s)

	var b strings.Builder
	b.Grow(len(decomposed))
	for _, r := range decomposed {
		// Mn is "mark, nonspacing" — the combining accents NFD just split
		// out. Everything else is kept.
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}

	// Recompose so that any character NFD split but which has no separate
	// combining mark (and any that survived) is in a canonical form.
	return norm.NFC.String(b.String())
}
