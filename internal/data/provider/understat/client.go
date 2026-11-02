// Package understat implements the source contract against understat.com's
// undocumented JSON endpoint: six men's top flights, with xG but no fouls,
// possession or corners (see docs/data-model.md).
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
	"github.com/JohnSBarden/footui/internal/trace"
)

const defaultBaseURL = "https://understat.com"

// CacheDirEnv names an opt-in dev cache of raw responses. Off by default:
// a refresh must not silently serve yesterday's file.
const CacheDirEnv = "FOOTUI_HTTP_CACHE"

// Retry policy. Kept short: `footui refresh --all` multiplies a dead
// host's retry cycle by every competition-season.
const (
	requestTimeout = 15 * time.Second
	maxAttempts    = 3
	baseBackoff    = 500 * time.Millisecond
	maxBackoff     = 4 * time.Second
)

// Client fetches and caches per-(league,season) payloads; one payload
// serves every Provider method for that league-season.
type Client struct {
	httpClient *http.Client
	baseURL    string
	minGap     time.Duration
	cacheDir   string
	// OnRetry, when set, is called before each retry.
	OnRetry func(attempt, of int, err error)

	mu        sync.Mutex
	lastFetch time.Time
	cache     map[string]*leagueData
	// players caches player pages, which cover every season.
	players map[string]*playerData
}

// NewClient constructs a Client with minGap between outbound requests.
func NewClient(minGap time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout, Transport: trace.Transport(nil)},
		baseURL:    defaultBaseURL,
		minGap:     minGap,
		cacheDir:   os.Getenv(CacheDirEnv),
		cache:      make(map[string]*leagueData),
		players:    make(map[string]*playerData),
	}
}

func cacheKey(leagueKey, season string) string {
	return leagueKey + ":" + season
}

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
		// Corrupt cache file: re-fetch, which overwrites it.
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

// diskCachePath normalizes league keys, which contain spaces ("La liga").
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

// writeDiskCache ignores failures; they shouldn't fail a good refresh.
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

func (c *Client) get(ctx context.Context, leagueKey, season string) ([]byte, error) {
	url := fmt.Sprintf("%s/getLeagueData/%s/%s", c.baseURL, leagueKey, season)
	return c.getURL(ctx, url, fmt.Sprintf("%s/league/%s", c.baseURL, leagueKey), leagueKey, season)
}

// getURL fetches url with retries. leagueKey and season only name the
// request in errors.
func (c *Client) getURL(ctx context.Context, url, referer, leagueKey, season string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Announced here so the final failure isn't reported as a retry.
			trace.Printf("understat: retry %d/%d of %s: %v", attempt, maxAttempts-1, url, lastErr)
			if c.OnRetry != nil {
				c.OnRetry(attempt, maxAttempts, lastErr)
			}
			if err := sleepCtx(ctx, backoffFor(attempt)); err != nil {
				return nil, err
			}
		}

		body, err := c.attempt(ctx, url, referer, leagueKey, season)
		if err == nil {
			return body, nil
		}
		lastErr = err

		if !retryable(ctx, err) {
			return nil, err
		}
	}

	trace.Printf("understat: giving up on %s after %d attempts: %v", url, maxAttempts, lastErr)
	return nil, &ExhaustedError{Attempts: maxAttempts, Last: lastErr}
}

func (c *Client) attempt(ctx context.Context, url, referer, leagueKey, season string) ([]byte, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("understat: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; footui; +https://github.com/JohnSBarden/footui)")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", referer)

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

// httpStatusError is a non-200 response.
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

// ExhaustedError is a fetch that failed every attempt. Last already names
// the league and season, so Error doesn't repeat them.
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
// timeout, replacing net/http's misleading "context deadline exceeded".
// It satisfies net.Error so retryable treats it as transient.
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

// retryable reports whether an error is transient. Cancellation is read
// from ctx, not err: net/http reports its own timeouts as
// context.DeadlineExceeded too, which would otherwise never be retried.
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
		// Retry a resolver hiccup, not "no such host".
		return dnsErr.IsTemporary || dnsErr.IsTimeout
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Unclassified errors are settled, so the endpoint isn't hammered.
	return false
}

// backoffFor returns the delay before attempt (1 is the first retry).
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
