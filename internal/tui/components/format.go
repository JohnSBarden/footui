package components

import (
	"fmt"
	"strconv"

	"github.com/JohnSBarden/footui/internal/data/models"
)

// Missing is printed for a stat the provider didn't supply; it must stay
// visually distinct from a genuine zero.
const Missing = "—"

// Float renders a nullable float to the given number of decimals.
func Float(v *float64, decimals int) string {
	if v == nil {
		return Missing
	}
	return strconv.FormatFloat(*v, 'f', decimals, 64)
}

// Ordinal is 1st, 2nd, 3rd, 4th … 11th, 12th, 13th, 21st.
func Ordinal(n int) string { return strconv.Itoa(n) + OrdinalSuffix(n) }

// OrdinalSuffix is the "st", "nd", "rd" or "th" that follows n.
func OrdinalSuffix(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// Int renders a nullable int.
func Int(v *int) string {
	if v == nil {
		return Missing
	}
	return strconv.Itoa(*v)
}

// Signed renders an int with an explicit sign.
func Signed(v int) string {
	if v > 0 {
		return "+" + strconv.Itoa(v)
	}
	return strconv.Itoa(v)
}

// SignedFloat renders a float with an explicit sign.
func SignedFloat(v float64, decimals int) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if v > 0 {
		return "+" + s
	}
	return s
}

// SeasonName displays a season the way its competition counts them:
// "2026" for a calendar-year league, "2026-27" for one that spans two.
func SeasonName(s models.Season) string {
	return SeasonLabelFor(s.CalendarYear, s.Label)
}

// SeasonLabelFor displays a bare start-year label as "2025" (calendar) or
// "2025-26"; anything else passes through untouched.
func SeasonLabelFor(calendar bool, label string) string {
	if calendar {
		return label
	}
	year, err := strconv.Atoi(label)
	if err != nil || year < 1888 || year > 2999 {
		return label
	}
	return fmt.Sprintf("%d-%02d", year, (year+1)%100)
}
