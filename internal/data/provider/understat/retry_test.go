package understat

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The distinction that matters is transient versus settled: retrying a
// timeout may work, retrying a 404 just multiplies the wait before the
// user learns the same thing.
func TestRetryableClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"503 service unavailable", &httpStatusError{Status: http.StatusServiceUnavailable}, true},
		{"500 internal error", &httpStatusError{Status: http.StatusInternalServerError}, true},
		{"429 rate limited", &httpStatusError{Status: http.StatusTooManyRequests}, true},
		{"404 not found", &httpStatusError{Status: http.StatusNotFound}, false},
		{"403 forbidden", &httpStatusError{Status: http.StatusForbidden}, false},
		{"no such host", &net.DNSError{Err: "no such host", IsNotFound: true}, false},
		{"temporary resolver failure", &net.DNSError{Err: "server misbehaving", IsTemporary: true}, true},
		{"dns timeout", &net.DNSError{Err: "timeout", IsTimeout: true}, true},
		{"user cancelled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"unclassified", errors.New("something odd"), false},
	}

	for _, c := range cases {
		if got := retryable(c.err); got != c.want {
			t.Errorf("%s: retryable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRetryableSeesThroughWrapping(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), &httpStatusError{Status: 503})
	if !retryable(wrapped) {
		t.Error("a wrapped 503 should still be retryable")
	}
}

func TestBackoffGrowsAndIsCapped(t *testing.T) {
	first := backoffFor(1)
	second := backoffFor(2)
	if second <= first {
		t.Errorf("backoff should grow: %v then %v", first, second)
	}
	// However many attempts, the wait must stay bounded — a full refresh
	// multiplies this by every competition-season in the run.
	for attempt := 1; attempt < 20; attempt++ {
		if got := backoffFor(attempt); got > maxBackoff {
			t.Fatalf("backoff for attempt %d = %v, exceeds the %v cap", attempt, got, maxBackoff)
		}
	}
}

func TestSleepCtxReturnsImmediatelyOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := sleepCtx(ctx, 10*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("a cancelled sleep took %v; the user's ctrl+c should be immediate", elapsed)
	}
}

func TestStatusErrorMessageNamesTheScope(t *testing.T) {
	err := &httpStatusError{Status: 503, LeagueKey: "EPL", Season: "2025", URL: "https://example/x"}
	msg := err.Error()
	for _, want := range []string{"EPL", "2025", "503"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q should mention %q", msg, want)
		}
	}
}
