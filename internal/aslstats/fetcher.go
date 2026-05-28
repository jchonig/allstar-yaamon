// Package aslstats fetches live node statistics from the AllStarLink stats API.
package aslstats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://stats.allstarlink.org"

// NodeStats holds the fetched statistics for one AllStar node.
type NodeStats struct {
	NodeNumber     string    `json:"node_number"`
	Callsign       string    `json:"callsign"`
	Description    string    `json:"description"`
	Location       string    `json:"location"`
	Keyed          bool      `json:"keyed"`
	TotalTxTime    float64   `json:"total_tx_time"` // seconds
	TotalKeyups    int       `json:"total_keyups"`
	LinkedNodes    []string  `json:"linked_nodes"`
	ConnectedLinks int       `json:"connected_links"`
	Web            bool      `json:"web"`
	FetchedAt      time.Time `json:"fetched_at"`
	Error          string    `json:"error,omitempty"`
}

// Stale returns true if the stats are older than 90 seconds.
func (s NodeStats) Stale() bool {
	return time.Since(s.FetchedAt) > 90*time.Second
}

// rateLimiter is a simple token-bucket rate limiter using a buffered channel.
type rateLimiter struct {
	tokens chan struct{}
}

func newRateLimiter(perMinute int) *rateLimiter {
	rl := &rateLimiter{tokens: make(chan struct{}, perMinute)}
	// Pre-fill with a small initial burst.
	burst := perMinute / 4
	if burst < 1 {
		burst = 1
	}
	for i := 0; i < burst; i++ {
		rl.tokens <- struct{}{}
	}
	// Refill one token per interval indefinitely.
	go func() {
		interval := time.Minute / time.Duration(perMinute)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case rl.tokens <- struct{}{}:
			default:
			}
		}
	}()
	return rl
}

func (r *rateLimiter) Wait(ctx context.Context) error {
	select {
	case <-r.tokens:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Fetcher fetches ASL node stats with a shared rate limiter across all goroutines.
type Fetcher struct {
	baseURL string
	client  *http.Client
	limiter *rateLimiter
}

// New creates a Fetcher. Pass an empty baseURL to use the production endpoint.
func New(baseURL string) *Fetcher {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Fetcher{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		limiter: newRateLimiter(10), // conservative: multiple instances may share an IP behind NAT
	}
}

// FetchDirect fetches stats for up to maxNodes node numbers concurrently using a
// semaphore instead of the shared rate limiter. Intended for on-demand user-initiated
// requests where rate-limit queuing would cause visible timeouts.
func (f *Fetcher) FetchDirect(ctx context.Context, nodeNumbers []string, maxNodes, concurrency int) map[string]NodeStats {
	if len(nodeNumbers) == 0 {
		return nil
	}
	if len(nodeNumbers) > maxNodes {
		nodeNumbers = nodeNumbers[:maxNodes]
	}
	sem := make(chan struct{}, concurrency)
	out := make(map[string]NodeStats, len(nodeNumbers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, num := range nodeNumbers {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			s := f.Fetch(ctx, n)
			mu.Lock()
			out[n] = s
			mu.Unlock()
		}(num)
	}
	wg.Wait()
	return out
}

// FetchAll fetches stats for all nodeNumbers concurrently, respecting the rate limit.
// The returned map contains an entry for every requested node number.
func (f *Fetcher) FetchAll(ctx context.Context, nodeNumbers []string) map[string]NodeStats {
	if len(nodeNumbers) == 0 {
		return nil
	}
	out := make(map[string]NodeStats, len(nodeNumbers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, num := range nodeNumbers {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			if err := f.limiter.Wait(ctx); err != nil {
				return
			}
			s := f.Fetch(ctx, n)
			mu.Lock()
			out[n] = s
			mu.Unlock()
		}(num)
	}
	wg.Wait()
	return out
}

// rawNodeEntry is the per-node JSON shape shared by both the individual
// (/api/stats/{node}) and bulk (/api/stats/) endpoints.
//
// Individual response: the root object IS a rawNodeEntry.
// Bulk response:       map[nodeNumber]rawNodeEntry.
type rawNodeEntry struct {
	Node struct {
		Callsign             string `json:"callsign"`
		AccessWebtransceiver string `json:"access_webtransceiver"` // "0" or "1"
		NodeFrequency        string `json:"node_frequency"`        // site/description
	} `json:"node"`
	Stats struct {
		Data struct {
			Keyed       bool            `json:"keyed"`
			TotalTxTime json.RawMessage `json:"totaltxtime"` // string or number
			TotalKeyups json.RawMessage `json:"totalkeyups"` // string or number
			Links       json.RawMessage `json:"links"`       // array or 0 when empty
		} `json:"data"`
	} `json:"stats"`
}

// parseEntry converts a rawNodeEntry into a NodeStats value.
func parseEntry(nodeNumber string, raw rawNodeEntry) NodeStats {
	s := NodeStats{
		NodeNumber:  nodeNumber,
		Callsign:    raw.Node.Callsign,
		Description: raw.Node.NodeFrequency,
		Web:         raw.Node.AccessWebtransceiver != "" && raw.Node.AccessWebtransceiver != "0",
		Keyed:       raw.Stats.Data.Keyed,
		FetchedAt:   time.Now(),
	}
	if v, err := strconv.ParseFloat(strings.Trim(string(raw.Stats.Data.TotalTxTime), `"`), 64); err == nil {
		s.TotalTxTime = v
	}
	if v, err := strconv.Atoi(strings.Trim(string(raw.Stats.Data.TotalKeyups), `"`)); err == nil {
		s.TotalKeyups = v
	}
	// links is an array of node number strings when connected, or the integer 0 when empty.
	if len(raw.Stats.Data.Links) > 0 && raw.Stats.Data.Links[0] == '[' {
		var links []string
		if json.Unmarshal(raw.Stats.Data.Links, &links) == nil {
			s.LinkedNodes = links
			s.ConnectedLinks = len(links)
		}
	}
	return s
}

// Fetch fetches stats for a single node number.
func (f *Fetcher) Fetch(ctx context.Context, nodeNumber string) NodeStats {
	url := fmt.Sprintf("%s/api/stats/%s", f.baseURL, nodeNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return NodeStats{NodeNumber: nodeNumber, Error: err.Error(), FetchedAt: time.Now()}
	}
	req.Header.Set("User-Agent", "YAAMon/1.0")
	resp, err := f.client.Do(req)
	if err != nil {
		slog.Warn("ASL stats fetch error", "node", nodeNumber, "err", err)
		return NodeStats{NodeNumber: nodeNumber, Error: err.Error(), FetchedAt: time.Now()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("ASL stats fetch error", "node", nodeNumber, "status", resp.StatusCode)
		return NodeStats{NodeNumber: nodeNumber, Error: fmt.Sprintf("HTTP %d", resp.StatusCode), FetchedAt: time.Now()}
	}
	var raw rawNodeEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		slog.Warn("ASL stats decode error", "node", nodeNumber, "err", err)
		return NodeStats{NodeNumber: nodeNumber, Error: err.Error(), FetchedAt: time.Now()}
	}
	return parseEntry(nodeNumber, raw)
}

// FetchBulk fetches stats for all currently-reporting nodes from the bulk endpoint
// (/api/stats/). Subject to a 1 request/minute/IP rate limit by AllStarLink.
// Returns every entry received; callers should cache all of them.
func (f *Fetcher) FetchBulk(ctx context.Context) (map[string]NodeStats, error) {
	url := f.baseURL + "/api/stats/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "YAAMon/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		slog.Warn("ASL stats bulk fetch error", "err", err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("ASL stats bulk fetch error", "status", resp.StatusCode)
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var raw map[string]rawNodeEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		slog.Warn("ASL stats bulk decode error", "err", err)
		return nil, err
	}

	now := time.Now()
	out := make(map[string]NodeStats, len(raw))
	for nodeNum, entry := range raw {
		s := parseEntry(nodeNum, entry)
		s.FetchedAt = now // consistent timestamp across the whole batch
		out[nodeNum] = s
	}
	slog.Info("ASL stats bulk fetch", "nodes", len(out))
	return out, nil
}
