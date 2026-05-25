package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/aslstats"
)

// statsCache is a thread-safe in-memory cache of fetched node stats.
type statsCache struct {
	mu    sync.RWMutex
	stats map[string]aslstats.NodeStats
}

func newStatsCache() *statsCache {
	return &statsCache{stats: make(map[string]aslstats.NodeStats)}
}

func (c *statsCache) get(nodeNumber string) (aslstats.NodeStats, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.stats[nodeNumber]
	return s, ok
}

func (c *statsCache) getMany(nodeNumbers []string) map[string]aslstats.NodeStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]aslstats.NodeStats, len(nodeNumbers))
	for _, n := range nodeNumbers {
		if s, ok := c.stats[n]; ok {
			out[n] = s
		}
	}
	return out
}

func (c *statsCache) update(results map[string]aslstats.NodeStats) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range results {
		c.stats[k] = v
	}
}

// startStatsPoller starts a background goroutine that fetches ASL stats on a ticker
// and pushes SSE updates to all subscribed dashboard clients.
func (s *Server) startStatsPoller(ctx context.Context) {
	go func() {
		s.pollStats(ctx)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.pollStats(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Server) pollStats(ctx context.Context) {
	nodes, err := s.db.ListNodes(ctx)
	if err != nil {
		return
	}

	// Collect unique favorite node numbers per home node.
	type nodeWork struct {
		id   int64
		nums []string
	}
	var work []nodeWork
	allNums := make(map[string]struct{})

	for _, n := range nodes {
		if !n.Enabled {
			continue
		}
		favs, err := s.db.ListFavoritesByNode(ctx, n.ID)
		if err != nil {
			continue
		}
		nums := make([]string, 0, len(favs))
		for _, f := range favs {
			nums = append(nums, f.NodeNumber)
			allNums[f.NodeNumber] = struct{}{}
		}
		if len(nums) > 0 {
			work = append(work, nodeWork{id: n.ID, nums: nums})
		}
	}

	if len(allNums) == 0 {
		return
	}

	unique := make([]string, 0, len(allNums))
	for n := range allNums {
		unique = append(unique, n)
	}

	results := s.fetcher.FetchAll(ctx, unique)
	s.statsCache.update(results)

	// Notify SSE subscribers per home node.
	for _, nw := range work {
		subset := make(map[string]aslstats.NodeStats, len(nw.nums))
		for _, num := range nw.nums {
			if st, ok := results[num]; ok {
				subset[num] = st
			}
		}
		data, err := json.Marshal(map[string]any{
			"type":   "stats",
			"nodeID": nw.id,
			"stats":  subset,
		})
		if err == nil {
			s.sseBroker.Publish(nw.id, data)
		}
	}
}

// handleSSE streams live stats updates for a node to the browser.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "nodeID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}

	// Send cached stats immediately as the first event.
	var initial []byte
	favs, err := s.db.ListFavoritesByNode(r.Context(), nodeID)
	if err == nil && len(favs) > 0 {
		nums := make([]string, len(favs))
		for i, f := range favs {
			nums[i] = f.NodeNumber
		}
		current := s.statsCache.getMany(nums)
		if len(current) > 0 {
			initial, _ = json.Marshal(map[string]any{
				"type":   "stats",
				"nodeID": nodeID,
				"stats":  current,
			})
		}
	}

	s.sseBroker.Stream(w, r, nodeID, initial)
}

// handleAPINodeStats returns the current cached stats for all favorites of a node.
func (s *Server) handleAPINodeStats(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	favs, err := s.db.ListFavoritesByNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	nums := make([]string, len(favs))
	for i, f := range favs {
		nums[i] = f.NodeNumber
	}
	writeJSON(w, s.statsCache.getMany(nums))
}

// handleConnect sends an AMI connect command.
// Body: {"target": "12345", "monitor": false}
func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	n, err := s.db.GetNodeByID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	var body struct {
		Target  string `json:"target"`
		Monitor bool   `json:"monitor"` // true = monitor-only (ilink 3), false = transceive (ilink 5)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Target == "" {
		http.Error(w, "body must be {\"target\":\"NNNNN\"}", http.StatusBadRequest)
		return
	}

	mode := "5" // transceive (default)
	if body.Monitor {
		mode = "3"
	}
	cmd := fmt.Sprintf("rpt cmd %s ilink %s %s", n.NodeNumber, mode, body.Target)
	if err := s.amiMgr.SendAction(nodeID, map[string]string{
		"Action":  "Command",
		"Command": cmd,
	}); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// handleDisconnect sends an AMI disconnect command.
// Body: {"target": "12345"} to disconnect one node, or {} to disconnect all.
func (s *Server) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	n, err := s.db.GetNodeByID(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	var body struct {
		Target string `json:"target"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck — empty body is valid (disconnect all)

	var cmd string
	if body.Target == "" {
		cmd = fmt.Sprintf("rpt cmd %s ilink 6", n.NodeNumber)
	} else {
		cmd = fmt.Sprintf("rpt cmd %s ilink 1 %s", n.NodeNumber, body.Target)
	}

	if err := s.amiMgr.SendAction(nodeID, map[string]string{
		"Action":  "Command",
		"Command": cmd,
	}); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

