package components

import (
	"testing"
	"time"
)

func TestFreshnessUsesTheCoarsestUsefulUnit(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{time.Minute, "1 minute ago"},
		{45 * time.Minute, "45 minutes ago"},
		{time.Hour, "1 hour ago"},
		{5 * time.Hour, "5 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{3 * 24 * time.Hour, "3 days ago"},
		{60 * 24 * time.Hour, "2 months ago"},
	}
	for _, c := range cases {
		if got := Freshness(now.Add(-c.ago), now); got != c.want {
			t.Errorf("Freshness(%v ago) = %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestFreshnessOfAnUnknownTimeIsEmpty(t *testing.T) {
	if got := Freshness(time.Time{}, time.Now()); got != "" {
		t.Errorf("a zero time should produce no text, got %q", got)
	}
}

// A clock change shouldn't make the UI claim data from the future.
func TestFreshnessNeverClaimsTheFuture(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if got := Freshness(now.Add(time.Hour), now); got != "just now" {
		t.Errorf("a future timestamp = %q, want a safe fallback", got)
	}
}
