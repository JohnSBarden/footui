// Package espn fetches competitions from ESPN's public JSON API: MLS,
// NWSL, the women's leagues, the European second divisions, the European
// club competitions, and international football — everything
// understat.com doesn't cover.
//
// The API is undocumented and keyless. It has no expected goals, and no
// minutes played per player, but it does have possession, corners, and
// fouls, which understat never had. Everything here is read-only and
// rate-limited; nothing is fetched that a refresh doesn't need.
package espn

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/JohnSBarden/footui/internal/data/provider"
)

const (
	siteBaseURL = "https://site.api.espn.com/apis"
	coreBaseURL = "https://sports.core.api.espn.com/v2/sports/soccer/leagues"
	// searchBaseURL is ESPN's site-wide search, which finds players
	// whatever they play in.
	searchBaseURL = "https://site.web.api.espn.com/apis/common/v3/search"

	// CacheDirEnv is shared with the understat client: one directory of
	// raw responses for development, so iterating doesn't hammer either
	// host.
	CacheDirEnv = "FOOTUI_HTTP_CACHE"

	requestTimeout = 15 * time.Second
	maxAttempts    = 3
	baseBackoff    = 500 * time.Millisecond
	// defaultMinGap spaces requests out. A league refresh is a roster and
	// a statistics request per club, so this is what keeps a 30-team
	// league from being 60 requests in a burst.
	defaultMinGap = 150 * time.Millisecond
	// scoreboardGap spaces out the big requests: a year of a competition's
	// matches is up to a couple of megabytes, and a nation's first
	// download is thirty of them.
	scoreboardGap = time.Second
	// maxRetryAfter caps how long a Retry-After header can make footui
	// wait in one go; a longer ask is treated as "not now" and gives up.
	maxRetryAfter = 2 * time.Minute
	// breakerOpen is how long every request is refused after ESPN kept
	// rate-limiting footui through all its retries.
	breakerOpen = time.Hour
)

// rateLimitBackoff is how long to wait after each successive 429 when
// ESPN doesn't say: generous, because a 429 means footui was already
// asking too much.
var rateLimitBackoff = []time.Duration{5 * time.Second, 20 * time.Second, 60 * time.Second}

// Client is a polite, retrying JSON fetcher for ESPN's two API hosts.
type Client struct {
	httpClient *http.Client
	siteURL    string
	coreURL    string
	searchURL  string
	minGap     time.Duration
	slowGap    time.Duration
	cacheDir   string
	// OnRetry, if set, is told about each failed attempt that will be
	// retried, so a slow refresh can say why it's slow.
	OnRetry func(attempt, of int, err error)

	// rateWaits is the 429 backoff schedule, and wait how a backoff is
	// slept; tests shorten both.
	rateWaits []time.Duration
	wait      func(ctx context.Context, d time.Duration) error
	breaker   *breaker

	mu        sync.Mutex
	lastFetch time.Time
}

// NewClient returns a client against the real API.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		siteURL:    siteBaseURL,
		coreURL:    coreBaseURL,
		searchURL:  searchBaseURL,
		minGap:     defaultMinGap,
		slowGap:    scoreboardGap,
		cacheDir:   os.Getenv(CacheDirEnv),
		rateWaits:  rateLimitBackoff,
		wait:       sleep,
		breaker:    sharedBreaker,
	}
}

// withBaseURLs points the client at a test server, with no pacing, no
// disk cache, and a breaker of its own.
func (c *Client) withBaseURLs(site, core string) *Client {
	c.siteURL, c.coreURL, c.minGap, c.slowGap, c.cacheDir = site, core, 0, 0, ""
	c.searchURL = site + "/search"
	c.rateWaits = []time.Duration{0, 0, 0}
	c.breaker = &breaker{}
	return c
}

// site fetches a path under the site API into out.
func (c *Client) site(ctx context.Context, path string, out any) error {
	return c.getJSON(ctx, c.siteURL+path, c.minGap, out)
}

// scoreboard fetches a scoreboard path under the site API into out, paced
// more widely than other requests.
func (c *Client) scoreboard(ctx context.Context, path string, out any) error {
	return c.getJSON(ctx, c.siteURL+path, c.slowGap, out)
}

// search runs a site-wide search query string into out.
func (c *Client) search(ctx context.Context, query string, out any) error {
	return c.getJSON(ctx, c.searchURL+"?"+query, c.minGap, out)
}

// core fetches a path under the core API into out.
func (c *Client) core(ctx context.Context, path string, out any) error {
	return c.getJSON(ctx, c.coreURL+path, c.minGap, out)
}

func (c *Client) getJSON(ctx context.Context, url string, gap time.Duration, out any) error {
	body, err := c.get(ctx, url, gap)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("espn: decode %s: %w", url, err)
	}
	return nil
}

// ErrNotFound is a 404: the competition, season, or team doesn't exist
// at ESPN. It is not retried.
var ErrNotFound = fmt.Errorf("espn: %w", provider.ErrNotFound)

// TimeoutError is a request that got no response in time.
type TimeoutError struct {
	URL   string
	After time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("espn: no response from %s within %s", e.URL, e.After)
}

// ExhaustedError wraps the last failure once every attempt is used up.
type ExhaustedError struct {
	Attempts int
	Last     error
}

func (e *ExhaustedError) Error() string {
	return fmt.Sprintf("espn: giving up after %d attempts: %v", e.Attempts, e.Last)
}

func (e *ExhaustedError) Unwrap() error { return e.Last }

// AttemptsMade implements provider.Exhausted.
func (e *ExhaustedError) AttemptsMade() int { return e.Attempts }

// TimedOutAfter implements provider.Timeout.
func (e *TimeoutError) TimedOutAfter() time.Duration { return e.After }

type statusError struct {
	URL    string
	Status int
	// RetryAfter is what a 429's Retry-After header asked for, zero when
	// it said nothing usable.
	RetryAfter time.Duration
}

func (e *statusError) Error() string {
	return fmt.Sprintf("espn: %s returned HTTP %d", e.URL, e.Status)
}

// errRateLimited is what every request gets once ESPN has kept refusing
// footui: no request is made at all.
var errRateLimited = fmt.Errorf("espn is rate-limiting footui; try again later: %w", provider.ErrRateLimited)

// get fetches url, retrying what is worth retrying. A 429 is handled on
// its own schedule: Retry-After when ESPN gives one, otherwise a backoff
// far longer than for a dropped connection, and if it keeps refusing,
// the breaker opens and nothing more is asked of it for an hour.
func (c *Client) get(ctx context.Context, url string, gap time.Duration) ([]byte, error) {
	if body, ok := c.readCache(url); ok {
		return body, nil
	}
	if c.breaker.open(time.Now()) {
		return nil, errRateLimited
	}
	var last error
	failures, refusals := 0, 0
	for {
		body, err := c.attempt(ctx, url, gap)
		if err == nil {
			c.writeCache(url, body)
			return body, nil
		}

		var se *statusError
		if errors.As(err, &se) && se.Status == http.StatusTooManyRequests && ctx.Err() == nil {
			if refusals == len(c.rateWaits) || se.RetryAfter > maxRetryAfter {
				c.breaker.trip(time.Now())
				return nil, errRateLimited
			}
			wait := c.rateWaits[refusals]
			if se.RetryAfter > 0 {
				wait = se.RetryAfter
			}
			refusals++
			if c.OnRetry != nil {
				c.OnRetry(refusals, len(c.rateWaits)+1, fmt.Errorf("ESPN asked footui to slow down; waiting %s", wait))
			}
			if err := c.wait(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}

		if !retryable(ctx, err) {
			return nil, err
		}
		failures++
		last = err
		if failures == maxAttempts {
			break
		}
		if c.OnRetry != nil {
			c.OnRetry(failures, maxAttempts, err)
		}
		if err := c.wait(ctx, baseBackoff<<(failures-1)); err != nil {
			return nil, err
		}
	}
	return nil, &ExhaustedError{Attempts: maxAttempts, Last: last}
}

func (c *Client) attempt(ctx context.Context, url string, gap time.Duration) ([]byte, error) {
	if err := c.throttle(ctx, gap); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "footui (github.com/JohnSBarden/footui)")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() && ctx.Err() == nil {
			return nil, &TimeoutError{URL: url, After: requestTimeout}
		}
		return nil, fmt.Errorf("espn: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, url)
	case resp.StatusCode != http.StatusOK:
		return nil, &statusError{URL: url, Status: resp.StatusCode,
			RetryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now())}
	}
	return provider.ReadBody(resp.Body)
}

// retryAfter reads a Retry-After header: seconds, or an HTTP date.
func retryAfter(h string, now time.Time) time.Duration {
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(h); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// retryable reports whether a failure is worth another attempt: timeouts,
// dropped connections, and server errors are; a 404, a 4xx, or the user
// cancelling are not. (A 429 has its own handling in get.)
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, ErrNotFound) {
		return false
	}
	var se *statusError
	if errors.As(err, &se) {
		return se.Status >= 500
	}
	return true
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// throttle waits until gap has passed since the previous request.
func (c *Client) throttle(ctx context.Context, gap time.Duration) error {
	c.mu.Lock()
	wait := gap - time.Since(c.lastFetch)
	c.mu.Unlock()
	if wait > 0 {
		if err := sleep(ctx, wait); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.lastFetch = time.Now()
	c.mu.Unlock()
	return nil
}

// breaker remembers that ESPN rate-limited footui through every retry, so
// that nothing else in the process asks again for a while. One is shared
// by every client: the TUI builds a provider per queued refresh, and the
// next one in the queue must not start the whole dance again.
type breaker struct {
	mu    sync.Mutex
	until time.Time
}

var sharedBreaker = &breaker{}

func (b *breaker) trip(now time.Time) {
	b.mu.Lock()
	b.until = now.Add(breakerOpen)
	b.mu.Unlock()
}

// PausedUntil is when ESPN may be asked again after it rate-limited footui
// through every retry, or zero when it isn't paused.
func PausedUntil() time.Time { return sharedBreaker.pausedUntil(time.Now()) }

func (b *breaker) pausedUntil(now time.Time) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if now.Before(b.until) {
		return b.until
	}
	return time.Time{}
}

func (b *breaker) open(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return now.Before(b.until)
}

func (c *Client) cachePath(url string) string {
	if c.cacheDir == "" {
		return ""
	}
	sum := sha1.Sum([]byte(url))
	return filepath.Join(c.cacheDir, "espn_"+hex.EncodeToString(sum[:])+".json")
}

func (c *Client) readCache(url string) ([]byte, bool) {
	p := c.cachePath(url)
	if p == "" {
		return nil, false
	}
	body, err := os.ReadFile(p)
	return body, err == nil
}

func (c *Client) writeCache(url string, body []byte) {
	if p := c.cachePath(url); p != "" {
		_ = os.MkdirAll(c.cacheDir, 0o755)
		_ = os.WriteFile(p, body, 0o644)
	}
}
