package components

import (
	"testing"

	"github.com/JohnSBarden/footui/internal/data/models"
)

func TestSeasonNameFollowsTheCompetitionsCalendar(t *testing.T) {
	if got := SeasonName(models.Season{Label: "2026", CalendarYear: true}); got != "2026" {
		t.Errorf("calendar-year season shown as %q, want 2026", got)
	}
	if got := SeasonName(models.Season{Label: "2026"}); got != "2026-27" {
		t.Errorf("split-year season shown as %q, want 2026-27", got)
	}
}

func TestSeasonLabelFormatsEuropeanSpan(t *testing.T) {
	cases := map[string]string{
		"2025": "2025-26",
		"2019": "2019-20",
		"1999": "1999-00",
		"2009": "2009-10",
	}
	for in, want := range cases {
		if got := SeasonLabelFor(false, in); got != want {
			t.Errorf("SeasonLabelFor(false, %q) = %q, want %q", in, got, want)
		}
	}
}

func TestSeasonLabelPassesThroughNonYears(t *testing.T) {
	for _, in := range []string{"2025-26", "", "apertura", "12"} {
		if got := SeasonLabelFor(false, in); got != in {
			t.Errorf("SeasonLabelFor(false, %q) = %q, want it unchanged", in, got)
		}
	}
}

func TestNullableStatsRenderAsMissingNotZero(t *testing.T) {
	if got := Float(nil, 1); got != Missing {
		t.Errorf("Float(nil) = %q, want %q", got, Missing)
	}
	if got := Int(nil); got != Missing {
		t.Errorf("Int(nil) = %q, want %q", got, Missing)
	}
	zero := 0.0
	if got := Float(&zero, 1); got != "0.0" {
		t.Errorf("Float(&0) = %q, want a real zero not %q", got, Missing)
	}
}

func TestSignedAlwaysShowsDirection(t *testing.T) {
	cases := map[int]string{5: "+5", -3: "-3", 0: "0"}
	for in, want := range cases {
		if got := Signed(in); got != want {
			t.Errorf("Signed(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateFitsWidthAndMarksTheCut(t *testing.T) {
	if got := Truncate("Wolverhampton Wanderers", 10); len([]rune(got)) != 10 {
		t.Errorf("Truncate(...,10) = %q, want exactly 10 cells", got)
	}
	if got := Truncate("Arsenal", 20); got != "Arsenal" {
		t.Errorf("Truncate should leave short strings alone, got %q", got)
	}
	if got := Truncate("Bayern München", 8); got != "Bayern M…" && len([]rune(got)) > 8 {
		t.Errorf("Truncate(%q, 8) = %q, too wide", "Bayern München", got)
	}
}

func TestPadAlignsToExactWidth(t *testing.T) {
	if got := pad("Pts", 6, true); got != "   Pts" {
		t.Errorf("pad right = %q", got)
	}
	if got := pad("Team", 8, false); got != "Team    " {
		t.Errorf("pad left = %q", got)
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 101: "101st", 111: "111th"} {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}
