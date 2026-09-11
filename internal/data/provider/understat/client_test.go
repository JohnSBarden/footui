package understat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureJSON = `{"teams":{"1":{"id":"1","title":"Arsenal","history":[
	{"xG":2.1,"xGA":0.5,"scored":2,"missed":0,"wins":1,"draws":0,"loses":0,"pts":3,
	 "ppda":{"att":100,"def":10},"ppda_allowed":{"att":50,"def":10},"deep":9,"deep_allowed":3,"xpts":2.4}
]}},"players":[]}`

// newTestClient points a client at a local server, with throttling off so
// tests don't pay the politeness delay.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c := NewClient(0)
	c.baseURL = srv.URL
	c.cacheDir = ""
	return c, srv
}

func TestFetchParsesAPayload(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/getLeagueData/EPL/2025") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write([]byte(fixtureJSON))
	}))

	data, err := c.fetchLeagueData(context.Background(), "EPL", "2025")
	if err != nil {
		t.Fatalf("fetchLeagueData: %v", err)
	}
	if len(data.Teams) != 1 {
		t.Errorf("got %d teams", len(data.Teams))
	}
}

// The provider satisfies several interfaces off one payload; each must not
// trigger its own round trip.
func TestRepeatedFetchesForOneSeasonHitTheNetworkOnce(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(fixtureJSON))
	}))

	for i := 0; i < 4; i++ {
		if _, err := c.fetchLeagueData(context.Background(), "EPL", "2025"); err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("made %d requests for one league/season, want 1", got)
	}
}

func TestDifferentSeasonsAreCachedSeparately(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(fixtureJSON))
	}))

	c.fetchLeagueData(context.Background(), "EPL", "2025")
	c.fetchLeagueData(context.Background(), "EPL", "2024")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("made %d requests for two seasons, want 2", got)
	}
}

func TestTransientFailureIsRetriedThenSucceeds(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(fixtureJSON))
	}))

	if _, err := c.fetchLeagueData(context.Background(), "EPL", "2025"); err != nil {
		t.Fatalf("a retryable failure should have recovered: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("made %d attempts, want 3", got)
	}
}

// Retrying a 404 just multiplies the wait before the user learns the
// season isn't there.
func TestSettledFailureIsNotRetried(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))

	if _, err := c.fetchLeagueData(context.Background(), "EPL", "1850"); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("made %d attempts at a 404, want 1", got)
	}
}

func TestPersistentFailureGivesUpAfterMaxAttempts(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))

	var retries int
	c.OnRetry = func(attempt, of int, err error) { retries++ }

	_, err := c.fetchLeagueData(context.Background(), "EPL", "2025")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&calls); got != maxAttempts {
		t.Errorf("made %d attempts, want the %d cap", got, maxAttempts)
	}
	if !strings.Contains(err.Error(), "giving up") {
		t.Errorf("error should say it exhausted its attempts: %v", err)
	}
	// The final failure is not followed by a retry, so it must not be
	// announced as one.
	if retries != maxAttempts-1 {
		t.Errorf("OnRetry fired %d times for %d attempts, want %d", retries, maxAttempts, maxAttempts-1)
	}
}

func TestMalformedPayloadIsAnError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>not json</html>"))
	}))

	if _, err := c.fetchLeagueData(context.Background(), "EPL", "2025"); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestDiskCacheAvoidsTheNetworkEntirely(t *testing.T) {
	dir := t.TempDir()
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Write([]byte(fixtureJSON))
	}))
	c.cacheDir = dir

	if _, err := c.fetchLeagueData(context.Background(), "EPL", "2025"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("first fetch made %d requests", got)
	}

	// A brand-new client shares only the directory, not the in-process
	// cache, so a second read proves the file was actually used.
	fresh := NewClient(0)
	fresh.baseURL = c.baseURL
	fresh.cacheDir = dir

	if _, err := fresh.fetchLeagueData(context.Background(), "EPL", "2025"); err != nil {
		t.Fatalf("cached fetch: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("disk cache was not used: %d total requests", got)
	}
}

// A corrupt cache file must not be fatal — re-fetch and overwrite it.
func TestCorruptDiskCacheFallsBackToTheNetwork(t *testing.T) {
	dir := t.TempDir()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fixtureJSON))
	}))
	c.cacheDir = dir

	path := c.diskCachePath("EPL", "2025")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{{{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := c.fetchLeagueData(context.Background(), "EPL", "2025")
	if err != nil {
		t.Fatalf("a corrupt cache file should fall through to the network: %v", err)
	}
	if len(data.Teams) != 1 {
		t.Errorf("got %d teams", len(data.Teams))
	}

	body, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(body), "not json") {
		t.Error("the corrupt cache file should have been overwritten")
	}
}

// League keys contain spaces ("La liga", "Serie A"), which cannot be used
// verbatim as path segments.
func TestDiskCachePathNormalizesLeagueKeys(t *testing.T) {
	c := NewClient(0)
	c.cacheDir = "/tmp/cache"

	for key, want := range map[string]string{
		"La liga":    "la-liga_2025.json",
		"Serie A":    "serie-a_2025.json",
		"EPL":        "epl_2025.json",
		"Bundesliga": "bundesliga_2025.json",
	} {
		if got := filepath.Base(c.diskCachePath(key, "2025")); got != want {
			t.Errorf("diskCachePath(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestNoCacheDirMeansNoDiskCache(t *testing.T) {
	c := NewClient(0)
	c.cacheDir = ""
	if got := c.diskCachePath("EPL", "2025"); got != "" {
		t.Errorf("expected no cache path without a directory, got %q", got)
	}
}

// An unwritable cache directory must not fail a refresh that otherwise
// worked — the caller already has the data.
func TestUnwritableCacheDirDoesNotFailTheFetch(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fixtureJSON))
	}))
	c.cacheDir = "/proc/definitely-not-writable"

	if _, err := c.fetchLeagueData(context.Background(), "EPL", "2025"); err != nil {
		t.Errorf("a failed cache write should not fail the fetch: %v", err)
	}
}

func TestThrottleSpacesOutRequests(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fixtureJSON))
	}))
	c.minGap = 150 * time.Millisecond

	start := time.Now()
	c.fetchLeagueData(context.Background(), "EPL", "2025")
	c.fetchLeagueData(context.Background(), "EPL", "2024")
	elapsed := time.Since(start)

	if elapsed < c.minGap {
		t.Errorf("two requests took %v; the politeness gap of %v was not applied", elapsed, c.minGap)
	}
}

func TestCancellationStopsAFetch(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fixtureJSON))
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.fetchLeagueData(ctx, "EPL", "2025"); err == nil {
		t.Error("a cancelled context should stop the fetch")
	}
}

func TestRequestCarriesTheHeadersTheEndpointExpects(t *testing.T) {
	var gotUA, gotXHR, gotReferer string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotXHR = r.Header.Get("X-Requested-With")
		gotReferer = r.Header.Get("Referer")
		w.Write([]byte(fixtureJSON))
	}))

	c.fetchLeagueData(context.Background(), "EPL", "2025")

	if gotUA == "" || !strings.Contains(gotUA, "footui") {
		t.Errorf("User-Agent should identify footui, got %q", gotUA)
	}
	if gotXHR != "XMLHttpRequest" {
		t.Errorf("X-Requested-With = %q", gotXHR)
	}
	if !strings.Contains(gotReferer, "/league/EPL") {
		t.Errorf("Referer = %q", gotReferer)
	}
}

// A request that times out is the most common transient failure there is,
// and net/http reports it as context.DeadlineExceeded — the same sentinel
// a user's ctrl+c produces. It must still be retried.
func TestRequestTimeoutIsRetried(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 2 {
			time.Sleep(200 * time.Millisecond)
			return
		}
		w.Write([]byte(fixtureJSON))
	}))
	c.httpClient.Timeout = 50 * time.Millisecond

	var retries []string
	c.OnRetry = func(attempt, of int, err error) {
		retries = append(retries, fmt.Sprintf("%d/%d %v", attempt, of, err))
	}

	if _, err := c.fetchLeagueData(context.Background(), "EPL", "2025"); err != nil {
		t.Fatalf("a timed-out request should have been retried and recovered: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("made %d attempts, want 2", got)
	}
	if len(retries) != 1 || !strings.HasPrefix(retries[0], "1/3 ") {
		t.Errorf("OnRetry calls = %q, want one for attempt 1 of 3", retries)
	}
}

// The user's interrupt looks identical to a timeout from the error alone;
// the difference is that their context is done.
func TestUserCancellationIsNotRetried(t *testing.T) {
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		cancel()
		time.Sleep(200 * time.Millisecond)
	}))

	if _, err := c.fetchLeagueData(ctx, "EPL", "2025"); err == nil {
		t.Fatal("expected an error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("made %d attempts after ctrl+c, want 1", got)
	}
}

// net/http describes its own timeout as "context deadline exceeded
// (Client.Timeout exceeded while awaiting headers)", which tells the user
// nothing about what happened or why. The error they read should.
func TestTimeoutErrorIsReadable(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	c.httpClient.Timeout = 50 * time.Millisecond

	_, err := c.fetchLeagueData(context.Background(), "EPL", "2025")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no response") || !strings.Contains(msg, "50ms") {
		t.Errorf("timeout should say no response arrived within the timeout: %q", msg)
	}
	if strings.Contains(msg, "Client.Timeout") || strings.Contains(msg, "context deadline") {
		t.Errorf("timeout should not leak net/http internals: %q", msg)
	}
	if strings.Count(msg, "EPL 2025") != 1 {
		t.Errorf("league/season should be named exactly once: %q", msg)
	}
}
