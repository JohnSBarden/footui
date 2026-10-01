// Package espn fetches competitions from ESPN's public JSON API: MLS,
// NWSL, the women's leagues, the European second divisions, and the
// European club competitions — everything understat.com doesn't cover.
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
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	siteBaseURL = "https://site.api.espn.com/apis"
	coreBaseURL = "https://sports.core.api.espn.com/v2/sports/soccer/leagues"

	// CacheDirEnv is shared with the understat client: one directory of
	// raw responses for development, so iterating doesn't hammer either
	// host.
	CacheDirEnv = "FOOTY_HTTP_CACHE"

	requestTimeout = 15 * time.Second
	maxAttempts    = 3
	baseBackoff    = 500 * time.Millisecond
	// defaultMinGap spaces requests out. A league refresh is a roster and
	// a statistics request per club, so this is what keeps a 30-team
	// league from being 60 requests in a burst.
	defaultMinGap = 150 * time.Millisecond
)

// Client is a polite, retrying JSON fetcher for ESPN's two API hosts.
type Client struct {
	httpClient *http.Client
	siteURL    string
	coreURL    string
	minGap     time.Duration
	cacheDir   string
	// OnRetry, if set, is told about each failed attempt that will be
	// retried, so a slow refresh can say why it's slow.
	OnRetry func(attempt, of int, err error)

	mu        sync.Mutex
	lastFetch time.Time
}

// NewClient returns a client against the real API.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		siteURL:    siteBaseURL,
		coreURL:    coreBaseURL,
		minGap:     defaultMinGap,
		cacheDir:   os.Getenv(CacheDirEnv),
	}
}

// withBaseURLs points the client at a test server.
func (c *Client) withBaseURLs(site, core string) *Client {
	c.siteURL, c.coreURL, c.minGap, c.cacheDir = site, core, 0, ""
	return c
}

// site fetches a path under the site API into out.
func (c *Client) site(ctx context.Context, path string, out any) error {
	return c.getJSON(ctx, c.siteURL+path, out)
}

// core fetches a path under the core API into out.
func (c *Client) core(ctx context.Context, path string, out any) error {
	return c.getJSON(ctx, c.coreURL+path, out)
}

func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	body, err := c.get(ctx, url)
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
var ErrNotFound = errors.New("espn: not found")

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

type statusError struct {
	URL    string
	Status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("espn: %s returned HTTP %d", e.URL, e.Status)
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	if body, ok := c.readCache(url); ok {
		return body, nil
	}
	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		body, err := c.attempt(ctx, url)
		if err == nil {
			c.writeCache(url, body)
			return body, nil
		}
		if !retryable(ctx, err) {
			return nil, err
		}
		last = err
		if attempt == maxAttempts {
			break
		}
		if c.OnRetry != nil {
			c.OnRetry(attempt, maxAttempts, err)
		}
		if err := sleep(ctx, baseBackoff<<(attempt-1)); err != nil {
			return nil, err
		}
	}
	return nil, &ExhaustedError{Attempts: maxAttempts, Last: last}
}

func (c *Client) attempt(ctx context.Context, url string) ([]byte, error) {
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "footy (github.com/JohnSBarden/footui)")
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
		return nil, &statusError{URL: url, Status: resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

// retryable reports whether a failure is worth another attempt: timeouts,
// dropped connections, and server errors are; a 404, a 4xx, or the user
// cancelling are not.
func retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, ErrNotFound) {
		return false
	}
	var se *statusError
	if errors.As(err, &se) {
		return se.Status >= 500 || se.Status == http.StatusTooManyRequests
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

func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := c.minGap - time.Since(c.lastFetch)
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
