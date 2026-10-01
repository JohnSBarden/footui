package textfold

import "testing"

func TestFoldStripsAccentsAndCase(t *testing.T) {
	cases := map[string]string{
		"Martín Zubimendi":    "martin zubimendi",
		"Jurriën Timber":      "jurrien timber",
		"Bayern München":      "bayern munchen",
		"Atlético Madrid":     "atletico madrid",
		"Borussia M.Gladbach": "borussia m.gladbach",
		"Arsenal":             "arsenal",
		"ARSENAL":             "arsenal",
		"Lisandro Martínez":   "lisandro martinez",
		"Nicolás Otamendi":    "nicolas otamendi",
		"Çağlar Söyüncü":      "caglar soyuncu",
		"Håland":              "haland",
		// Letters with no combining mark for NFD to split off.
		"Martin Ødegaard":   "martin odegaard",
		"Rasmus Højlund":    "rasmus hojlund",
		"Kevin Großkreutz":  "kevin grosskreutz",
		"Łukasz Fabiański":  "lukasz fabianski",
		"Kerem Aktürkoğlu":  "kerem akturkoglu",
		"Arda Güler (ı)":    "arda guler (i)",
		"Søren Kierkegaard": "soren kierkegaard",
		"Đorđe Petrović":    "dorde petrovic",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

// The whole point: an ASCII query and the accented name must fold to the
// same thing, in both directions.
func TestASCIIQueriesMatchAccentedNames(t *testing.T) {
	pairs := [][2]string{
		{"martin", "Martín"},
		{"MARTIN", "Martín"},
		{"Martín", "martin"},
		{"jurrien", "Jurriën"},
		{"munchen", "München"},
	}
	for _, p := range pairs {
		q, name := Fold(p[0]), Fold(p[1])
		if !contains(name, q) && !contains(q, name) {
			t.Errorf("Fold(%q)=%q and Fold(%q)=%q don't match", p[0], q, p[1], name)
		}
	}
}

func TestFoldIsIdempotent(t *testing.T) {
	for _, s := range []string{"Martín Zubimendi", "Bayern München", "Arsenal", ""} {
		once := Fold(s)
		if twice := Fold(once); twice != once {
			t.Errorf("Fold(%q) is not idempotent: %q then %q", s, once, twice)
		}
	}
}

func TestFoldLeavesNonLatinScriptsSearchable(t *testing.T) {
	// Cyrillic has no accents to strip; folding must still lowercase it
	// rather than mangling or dropping it.
	if got := Fold("Зенит"); got != "зенит" {
		t.Errorf("Fold(Cyrillic) = %q", got)
	}
}

func TestFoldOfEmptyStringIsEmpty(t *testing.T) {
	if got := Fold(""); got != "" {
		t.Errorf("Fold(\"\") = %q", got)
	}
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
