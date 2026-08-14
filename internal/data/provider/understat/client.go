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
	"net/http"
	"sync"
	"time"
)

const baseURL = "https://understat.com"

// Client fetches and caches per-(league,season) payloads from
// understat.com. A single payload contains everything needed to satisfy
// StandingsProvider, TeamStatsProvider, XGProvider, and PlayerStatsProvider
// for that league/season, so callers of different interface methods for the
// same league/season only trigger one HTTP fetch.
type Client struct {
	httpClient *http.Client
	minGap     time.Duration

	mu        sync.Mutex
	lastFetch time.Time
	cache     map[string]*leagueData
}

// NewClient constructs a Client. minGap is the minimum delay enforced
// between outbound requests (politeness towards an undocumented endpoint).
func NewClient(minGap time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		minGap:     minGap,
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
		return nil, fmt.Errorf("understat: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("understat: fetch %s: unexpected status %d", url, resp.StatusCode)
	}

	var payload leagueData
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("understat: decode %s: %w", url, err)
	}

	c.mu.Lock()
	c.cache[key] = &payload
	c.mu.Unlock()

	return &payload, nil
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
