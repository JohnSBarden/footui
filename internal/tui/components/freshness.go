package components

import (
	"fmt"
	"time"
)

// Freshness renders how long ago a refresh happened, in the coarsest unit
// that still says something useful. Cached football data is meaningfully
// stale after days, not seconds, so "3 days ago" is the answer a user
// wants and "3 days, 4 hours, 12 minutes ago" is noise.
func Freshness(then, now time.Time) string {
	if then.IsZero() {
		return ""
	}

	d := now.Sub(then)
	switch {
	case d < 0:
		// A clock change or a hand-edited row; claiming the future would
		// be worse than saying nothing definite.
		return "just now"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return Plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return Plural(int(d.Hours()), "hour") + " ago"
	case d < 30*24*time.Hour:
		return Plural(int(d.Hours()/24), "day") + " ago"
	default:
		return Plural(int(d.Hours()/24/30), "month") + " ago"
	}
}

// Plural is "1 goal" or "3 goals".
func Plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
