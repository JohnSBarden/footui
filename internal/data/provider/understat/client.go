// Package understat implements the source contract (internal/data/provider)
// against understat.com's internal JSON endpoint. It is footui's v1 data
// provider.
//
// It was substituted in for the FBref scraper described in the original
// design during implementation: FBref sits behind a Cloudflare JS
// challenge that blocks plain HTTP fetches (and the sandbox this was built
// in has no headless-browser access to clear it), while understat.com
// serves the same class of data as a fetchable, undocumented-but-stable
// JSON endpoint. See docs/data-model.md for the resulting coverage gap
// (understat.com covers 6 men's top-flight leagues only, and has no
// fouls/possession/corners data) and how another provider can close it
// later without touching anything outside this package.
package understat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
)

// defaultBaseURL is understat.com. It is a Client field rather than a
// package constant so tests can point the client at a local server and
// exercise the retry, throttle, and cache paths without a network.
const defaultBaseURL = "https://understat.com"

// CacheDirEnv names an optional directory for caching raw endpoint
// responses on disk. When it's set, a league/season payload is read from
// there if present and written there after a successful fetch.
//
// This is deliberately opt-in and off by default: `footui refresh` means
// "go get the current numbers", and silently serving yesterday's file
// would make that a lie. It exists so that developing against the data
// (and running the opt-in live tests) doesn't hammer an undocumented
// third-party endpoint with identical requests.
const CacheDirEnv = "FOOTUI_HTTP_CACHE"

// Retry policy. The request timeout is kept short enough that a full
// retry cycle against a dead host costs well under a minute, because
// `footui refresh --all` multiplies whatever this is by every
// competition-season in the run.
const (
	requestTimeout = 15 * time.Second
	maxAttempts    = 3
	baseBackoff    = 500 * time.Millisecond
	maxBackoff     = 4 * time.Second
)

// Client fetches and caches per-(league,season) payloads from
// understat.com. A single payload holds everything Provider serves for that league/season,
// so calls to its different methods for the same league/season trigger only
// one HTTP fetch.
type Client struct {
	httpClient *http.Client
	baseURL    string
	minGap     time.Duration
	// cacheDir, when non-empty, is a directory of raw JSON responses read
	// before hitting the network and written after a successful fetch.
	cacheDir string
	// OnRetry, when set, is called before each retry. A refresh against an
	// unresponsive host is otherwise silent for the length of every
	// timeout it waits through, which is indistinguishable from a hang.
	OnRetry func(attempt, of int, err error)

	mu        sync.Mutex
	lastFetch time.Time
	cache     map[string]*leagueData
}

// NewClient constructs a Client. minGap is the minimum delay enforced
// between outbound requests (politeness towards an undocumented endpoint).
func NewClient(minGap time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    defaultBaseURL,
		minGap:     minGap,
		cacheDir:   os.Getenv(CacheDirEnv),
		cache:      make(map[string]*leagueData),
	}
}

func cacheKey(leagueKey, season string) string {
	return leagueKey + ":" + season
}

// fetchLeagueData returns the parsed payload for a league/season, using the
// in-memory cache when available.
func (c *Client) fetchLeagueData(ctx context.Context, leagueKey, season string) (*leagueData, error) {
	key := cacheKey(leagueKey, season)

	c.mu.Lock()
	if cached, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	if body, ok := c.readDiskCache(leagueKey, season); ok {
		var payload leagueData
		if err := json.Unmarshal(body, &payload); err == nil {
			c.mu.Lock()
			c.cache[key] = &payload
			c.mu.Unlock()
			return &payload, nil
		}
		// A corrupt cache file is not worth failing over: fall through and
		// re-fetch, which also overwrites the bad file.
	}

	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	body, err := c.get(ctx, leagueKey, season)
	if err != nil {
		return nil, err
	}

	var payload leagueData
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("understat: decode %s %s: %w", leagueKey, season, err)
	}

	c.writeDiskCache(leagueKey, season, body)

	c.mu.Lock()
	c.cache[key] = &payload
	c.mu.Unlock()

	return &payload, nil
}

var cacheNameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// diskCachePath maps a league/season to a cache filename. League keys
// contain spaces ("La liga", "Serie A"), so they're normalized rather than
// used verbatim as path segments.
func (c *Client) diskCachePath(leagueKey, season string) string {
	if c.cacheDir == "" {
		return ""
	}
	name := strings.Trim(cacheNameUnsafe.ReplaceAllString(strings.ToLower(leagueKey), "-"), "-")
	return filepath.Join(c.cacheDir, fmt.Sprintf("%s_%s.json", name, season))
}

func (c *Client) readDiskCache(leagueKey, season string) ([]byte, bool) {
	path := c.diskCachePath(leagueKey, season)
	if path == "" {
		return nil, false
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return body, true
}

// writeDiskCache persists a successful response. Cache write failures are
// ignored: the caller already has the data, and a read-only or full cache
// directory shouldn't fail a refresh that otherwise worked.
func (c *Client) writeDiskCache(leagueKey, season string, body []byte) {
	path := c.diskCachePath(leagueKey, season)
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o600)
}

// get fetches a league/season payload, retrying transient failures.
func (c *Client) get(ctx context.Context, leagueKey, season string) ([]byte, error) {
	url := fmt.Sprintf("%s/getLeagueData/%s/%s", c.baseURL, leagueKey, season)

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Announced here rather than after the failure so the last
			// attempt, which has no retry following it, isn't reported
			// as "trying again".
			if c.OnRetry != nil {
				c.OnRetry(attempt, maxAttempts, lastErr)
			}
			if err := sleepCtx(ctx, backoffFor(attempt)); err != nil {
				return nil, err
			}
		}

		body, err := c.attempt(ctx, url, leagueKey, season)
		if err == nil {
			return body, nil
		}
		lastErr = err

		if !retryable(ctx, err) {
			return nil, err
		}
	}

	return nil, &ExhaustedError{Attempts: maxAttempts, Last: lastErr}
}

func (c *Client) attempt(ctx context.Context, url, leagueKey, season string) ([]byte, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("understat: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; footui; +https://github.com/JohnSBarden/footui)")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", fmt.Sprintf("%s/league/%s", c.baseURL, leagueKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		var netErr net.Error
		if ctx.Err() == nil && errors.As(err, &netErr) && netErr.Timeout() {
			return nil, &TimeoutError{
				LeagueKey: leagueKey,
				Season:    season,
				Host:      req.URL.Host,
				After:     c.httpClient.Timeout,
			}
		}
		return nil, fmt.Errorf("understat: fetch %s %s: %w", leagueKey, season, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{
			Status:    resp.StatusCode,
			LeagueKey: leagueKey,
			Season:    season,
			URL:       url,
		}
	}

	body, err := provider.ReadBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("understat: read %s %s: %w", leagueKey, season, err)
	}
	return body, nil
}

// httpStatusError is a non-200 response, kept as a type so retryability
// can be decided on the status code rather than by matching error strings.
type httpStatusError struct {
	Status    int
	LeagueKey string
	Season    string
	URL       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("understat: fetch %s %s: unexpected status %d from %s",
		e.LeagueKey, e.Season, e.Status, e.URL)
}

// ExhaustedError is a fetch that failed on every attempt it was allowed.
// Last is the final attempt's error, which already names the league and
// season — repeating them here would double the prefix in the message
// the user reads.
type ExhaustedError struct {
	Attempts int
	Last     error
}

func (e *ExhaustedError) Error() string {
	return fmt.Sprintf("giving up after %d attempts: %v", e.Attempts, e.Last)
}

func (e *ExhaustedError) Unwrap() error { return e.Last }

// AttemptsMade implements provider.Exhausted.
func (e *ExhaustedError) AttemptsMade() int { return e.Attempts }

// TimedOutAfter implements provider.Timeout.
func (e *TimeoutError) TimedOutAfter() time.Duration { return e.After }

// TimeoutError is a request that got no response within the client's
// timeout. It is exported so a caller can recognise it and phrase the
// failure for its own medium. net/http's own wording for this is "context deadline exceeded
// (Client.Timeout exceeded while awaiting headers)", which is both
// misleading (no context of ours expired) and unreadable; this says what
// actually happened. It still satisfies net.Error so retryable treats it
// as the transient failure it is.
type TimeoutError struct {
	LeagueKey string
	Season    string
	Host      string
	After     time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("understat: fetch %s %s: no response from %s within %s",
		e.LeagueKey, e.Season, e.Host, e.After)
}

func (e *TimeoutError) Timeout() bool   { return true }
func (e *TimeoutError) Temporary() bool { return true }

// retryable reports whether an error is worth another attempt.
//
// The distinction that matters is transient versus settled. A timeout or a
// 503 may well succeed a second later. A 404 means the league/season
// genuinely isn't there, and a DNS failure means the host can't be
// resolved at all — retrying either just multiplies the wait before the
// user learns the same thing.
//
// Whether the user interrupted is read from ctx, not from err. net/http
// reports its own request timeouts as context.DeadlineExceeded as well, so
// classifying on the error alone treats every timeout as a cancellation
// and never retries the single most common transient failure — which is
// exactly the bug this signature exists to prevent.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}

	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status == http.StatusTooManyRequests || statusErr.Status >= 500
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		// A temporary resolver hiccup is worth retrying; "no such host"
		// is not.
		return dnsErr.IsTemporary || dnsErr.IsTimeout
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Anything else (a malformed request, a read failure) is treated as
	// settled: retrying an unclassified error risks hammering the endpoint
	// for no reason.
	return false
}

// backoffFor returns the delay before the given attempt (1-indexed after
// the first), growing exponentially.
func backoffFor(attempt int) time.Duration {
	d := baseBackoff << (attempt - 1)
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := c.minGap - time.Since(c.lastFetch)
	c.mu.Unlock()

	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	c.mu.Lock()
	c.lastFetch = time.Now()
	c.mu.Unlock()
	return nil
}
