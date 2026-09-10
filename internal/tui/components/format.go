package components

import (
	"fmt"
	"strconv"
)

// Missing is what every screen prints for a stat the provider didn't
// supply. understat.com has real gaps (no possession, fouls, or corners),
// so "not available" has to be visually distinct from a genuine zero.
const Missing = "—"

// Float renders a nullable float to the given number of decimals.
func Float(v *float64, decimals int) string {
	if v == nil {
		return Missing
	}
	return strconv.FormatFloat(*v, 'f', decimals, 64)
}

// Int renders a nullable int.
func Int(v *int) string {
	if v == nil {
		return Missing
	}
	return strconv.Itoa(*v)
}

// Signed renders an int with an explicit sign, for goal difference and
// xG-minus-actual style deltas where the direction matters as much as the
// magnitude.
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

// SeasonLabel converts a stored season label into its display form.
// Seasons are stored as the bare start year understat.com keys on
// ("2025"); a European season spanning two calendar years reads as
// "2025-26". A label that isn't a bare year (a future provider might use
// "2025" for a single-calendar-year league like MLS, or store "2025-26"
// directly) is passed through untouched.
func SeasonLabel(label string) string {
	year, err := strconv.Atoi(label)
	if err != nil || year < 1888 || year > 2999 {
		return label
	}
	return fmt.Sprintf("%d-%02d", year, (year+1)%100)
}

// Minutes renders a minutes-played total as-is; kept as a named helper so
// the unit suffix is defined in one place.
func Minutes(m int) string {
	return strconv.Itoa(m) + "'"
}
