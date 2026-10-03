package dashboard

import "time"

// PinClock fixes the time freshness is measured against.
func PinClock(t time.Time) { now = func() time.Time { return t } }
