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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const baseURL = "https://understat.com"

// CacheDirEnv names an optional directory for caching raw endpoint
// responses on disk. When it's set, a league/season payload is read from
// there if present and written there after a successful fetch.
//
// This is deliberately opt-in and off by default: `footy refresh` means
// "go get the current numbers", and silently serving yesterday's file
// would make that a lie. It exists so that developing against the data
// (and running the opt-in live tests) doesn't hammer an undocumented
// third-party endpoint with identical requests.
const CacheDirEnv = "FOOTY_HTTP_CACHE"

// Client fetches and caches per-(league,season) payloads from
// understat.com. A single payload contains everything needed to satisfy
// StandingsProvider, TeamStatsProvider, XGProvider, and PlayerStatsProvider
// for that league/season, so callers of different interface methods for the
// same league/season only trigger one HTTP fetch.
type Client struct {
	httpClient *http.Client
	minGap     time.Duration
	// cacheDir, when non-empty, is a directory of raw JSON responses read
	// before hitting the network and written after a successful fetch.
	cacheDir string

	mu        sync.Mutex
	lastFetch time.Time
	cache     map[string]*leagueData
}

// NewClient constructs a Client. minGap is the minimum delay enforced
// between outbound requests (politeness towards an undocumented endpoint).
func NewClient(minGap time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		minGap:     minGap,
		cacheDir:   os.Getenv(CacheDirEnv),
		cache:      make(map[string]*leagueData),
	}
}

// WithCacheDir returns a copy of the client that reads and writes raw
// responses under dir, overriding the environment variable.
func (c *Client) WithCacheDir(dir string) *Client {
	clone := NewClient(c.minGap)
	clone.cacheDir = dir
	return clone
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

	url := fmt.Sprintf("%s/getLeagueData/%s/%s", baseURL, leagueKey, season)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("understat: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; footui/0.1; +https://github.com/JohnSBarden/footui)")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", fmt.Sprintf("%s/league/%s", baseURL, leagueKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("understat: fetch %s %s: %w", leagueKey, season, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("understat: fetch %s %s: unexpected status %d from %s", leagueKey, season, resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("understat: read %s %s: %w", leagueKey, season, err)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o644)
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
