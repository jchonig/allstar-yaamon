// Package aslstats fetches live node statistics from the AllStarLink stats API.
package aslstats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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
		limiter: newRateLimiter(28), // conservative: < ASL's 30/min limit
	}
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

// Fetch fetches stats for a single node number.
func (f *Fetcher) Fetch(ctx context.Context, nodeNumber string) NodeStats {
	s := NodeStats{NodeNumber: nodeNumber, FetchedAt: time.Now()}
	url := fmt.Sprintf("%s/api/stats/%s", f.baseURL, nodeNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	resp, err := f.client.Do(req)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return s
	}

	// Actual response shape (stats.allstarlink.org/api/stats/{node}):
	// {
	//   "node": { "callsign": "W1AW", "access_webtransceiver": "1",
	//             "node_frequency": "Site description" },
	//   "stats": {
	//     "data": {
	//       "keyed": false,
	//       "totaltxtime": "6001",   // string-encoded seconds
	//       "totalkeyups": "776",    // string-encoded count
	//       "links": ["12345", "67890"]  // connected node numbers
	//     }
	//   }
	// }
	var raw struct {
		Node struct {
			Callsign             string `json:"callsign"`
			AccessWebtransceiver string `json:"access_webtransceiver"` // "0" or "1"
			NodeFrequency        string `json:"node_frequency"`        // site/description
		} `json:"node"`
		Stats struct {
			Data struct {
				Keyed       bool     `json:"keyed"`
				TotalTxTime string   `json:"totaltxtime"` // string-encoded seconds
				TotalKeyups string   `json:"totalkeyups"` // string-encoded count
				Links       []string `json:"links"`
			} `json:"data"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		s.Error = err.Error()
		return s
	}

	s.Callsign = raw.Node.Callsign
	s.Description = raw.Node.NodeFrequency
	s.Web = raw.Node.AccessWebtransceiver != "" && raw.Node.AccessWebtransceiver != "0"
	s.Keyed = raw.Stats.Data.Keyed
	if v, err := strconv.ParseFloat(raw.Stats.Data.TotalTxTime, 64); err == nil {
		s.TotalTxTime = v
	}
	if v, err := strconv.Atoi(raw.Stats.Data.TotalKeyups); err == nil {
		s.TotalKeyups = v
	}
	s.LinkedNodes = raw.Stats.Data.Links
	s.ConnectedLinks = len(raw.Stats.Data.Links)
	return s
}
