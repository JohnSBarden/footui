package espn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
)

// limitedServer answers 429 to its first `refuse` requests (with
// retryAfter as the header, if set), then 200.
func limitedServer(t *testing.T, refuse int32, retryAfter string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) <= refuse {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// recordWaits logs the client's sleeps instead of sleeping.
func recordWaits(c *Client) *[]time.Duration {
	var waits []time.Duration
	c.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	return &waits
}

func TestA429WaitsAsLongAsRetryAfterSays(t *testing.T) {
	srv, hits := limitedServer(t, 1, "7")
	c := NewClient().withBaseURLs(srv.URL, srv.URL)
	waits := recordWaits(c)
	var out map[string]bool
	if err := c.site(ctxBkgnd, "/x", &out); err != nil || !out["ok"] {
		t.Fatalf("got %v, %v", out, err)
	}
	if *hits != 2 || len(*waits) != 1 || (*waits)[0] != 7*time.Second {
		t.Errorf("%d requests, waits %v: want one 7s wait, as Retry-After asked", *hits, *waits)
	}
}

func TestA429WithoutRetryAfterBacksOffSlowly(t *testing.T) {
	srv, _ := limitedServer(t, 3, "")
	c := NewClient().withBaseURLs(srv.URL, srv.URL)
	c.rateWaits = rateLimitBackoff
	waits := recordWaits(c)
	var out map[string]bool
	if err := c.site(ctxBkgnd, "/x", &out); err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{5 * time.Second, 20 * time.Second, 60 * time.Second}
	if len(*waits) != 3 || (*waits)[0] != want[0] || (*waits)[1] != want[1] || (*waits)[2] != want[2] {
		t.Errorf("waits %v, want %v", *waits, want)
	}
}

// A 429 that outlasts every retry trips the shared breaker for an hour.
func TestPersistentRateLimitingOpensTheBreaker(t *testing.T) {
	srv, hits := limitedServer(t, 1000, "")
	c := NewClient().withBaseURLs(srv.URL, srv.URL)
	recordWaits(c)
	var out any
	err := c.site(ctxBkgnd, "/x", &out)
	if !errors.Is(err, provider.ErrRateLimited) {
		t.Fatalf("got %v, want ErrRateLimited", err)
	}
	if *hits != int32(len(c.rateWaits)+1) {
		t.Errorf("%d requests before giving up, want %d", *hits, len(c.rateWaits)+1)
	}

	before := *hits
	other := NewClient().withBaseURLs(srv.URL, srv.URL)
	other.breaker = c.breaker
	for _, client := range []*Client{c, other} {
		if err := client.site(ctxBkgnd, "/y", &out); !errors.Is(err, provider.ErrRateLimited) {
			t.Errorf("with the breaker open: %v", err)
		}
	}
	if *hits != before {
		t.Errorf("%d requests made with the breaker open", *hits-before)
	}
	if c.breaker.open(time.Now().Add(breakerOpen + time.Second)) {
		t.Error("the breaker should close after an hour")
	}
}

func TestAnAbsurdRetryAfterGivesUpRatherThanWaiting(t *testing.T) {
	srv, hits := limitedServer(t, 1000, "86400")
	c := NewClient().withBaseURLs(srv.URL, srv.URL)
	waits := recordWaits(c)
	var out any
	if err := c.site(ctxBkgnd, "/x", &out); !errors.Is(err, provider.ErrRateLimited) {
		t.Fatalf("got %v", err)
	}
	if *hits != 1 || len(*waits) != 0 {
		t.Errorf("%d requests, waits %v: a day's wait means stop now", *hits, *waits)
	}
}

func TestScoreboardsArePacedWiderThanOtherCalls(t *testing.T) {
	c := NewClient()
	if c.slowGap < time.Second || c.minGap >= c.slowGap {
		t.Errorf("gaps %s / %s: scoreboards need at least a second between them", c.minGap, c.slowGap)
	}
	if c.breaker != sharedBreaker {
		t.Error("real clients must share one breaker")
	}
}

func TestRetryAfterParsesSecondsAndDates(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for h, want := range map[string]time.Duration{
		"":                              0,
		"30":                            30 * time.Second,
		"Tue, 13 Oct 2026 12:01:00 GMT": time.Minute,
		"soon":                          0,
	} {
		if got := retryAfter(h, now); got != want {
			t.Errorf("Retry-After %q: %s, want %s", h, got, want)
		}
	}
}
