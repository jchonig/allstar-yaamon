package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
		if v.Error == "" {
			c.stats[k] = v
		}
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
	for _, n := range nodes {
		if n.Enabled && s.sseBroker.HasSubscribers(n.ID) {
			s.pollNodeStats(ctx, n.ID, false)
		}
	}
}

// pollNodeStats fetches fresh ASL stats for a single node's favorites, updates
// the cache, and publishes an SSE event. Called both by the periodic poller
// (for nodes with active viewers) and immediately when a client connects.
// When direct is true (SSE connect path) the shared rate limiter is bypassed
// so the first load is fast; periodic background polls use the rate limiter.
func (s *Server) pollNodeStats(ctx context.Context, nodeID int64, direct bool) {
	n, err := s.db.GetNodeByID(ctx, nodeID)
	if err != nil {
		return
	}
	favs, err := s.db.ListFavoritesByNode(ctx, nodeID)
	if err != nil {
		return
	}
	// Always include the home node's own number plus all favorites.
	seen := make(map[string]bool)
	nums := make([]string, 0, len(favs)+1)
	add := func(num string) {
		if !seen[num] {
			seen[num] = true
			nums = append(nums, num)
		}
	}
	add(n.NodeNumber)
	for _, f := range favs {
		add(f.NodeNumber)
	}
	// Also include currently active linked nodes so the Active Links panel
	// always shows fresh stats (connected_links, callsign) without waiting
	// for the node to appear in favorites.
	if linked, ok := s.linksCache.get(nodeID); ok {
		for nodeNum := range linked {
			add(nodeNum)
		}
	}
	var results map[string]aslstats.NodeStats
	if direct {
		results = s.fetcher.FetchDirect(ctx, nums, len(nums), 8)
	} else {
		results = s.fetcher.FetchAll(ctx, nums)
	}
	s.statsCache.update(results)

	subset := make(map[string]aslstats.NodeStats, len(nums))
	for _, num := range nums {
		if st, ok := results[num]; ok && st.Error == "" {
			subset[num] = st
		}
	}
	data, err := json.Marshal(map[string]any{
		"type":   "stats",
		"nodeID": nodeID,
		"stats":  subset,
	})
	if err == nil {
		s.sseBroker.Publish(nodeID, data)
	}
	slog.Debug("stats poll", "node_id", nodeID, "count", len(subset))
}

// handleSSE streams live stats updates for a node to the browser.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "nodeID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}

	// Subscribe before triggering the fresh fetch so no published events are
	// dropped between the fetch completing and the stream starting.
	ch, cancel := s.sseBroker.Subscribe(nodeID)
	defer cancel()

	// Trigger an immediate fresh stats fetch for this node in the background.
	// Because we subscribed above, the result will arrive on ch.
	go s.pollNodeStats(r.Context(), nodeID, true)

	// Send cached state as the initial SSE burst while the fresh fetch is in flight.
	var initial [][]byte
	{
		hn, hnerr := s.db.GetNodeByID(r.Context(), nodeID)
		favs, ferr := s.db.ListFavoritesByNode(r.Context(), nodeID)
		nums := make([]string, 0)
		if hnerr == nil {
			nums = append(nums, hn.NodeNumber)
		}
		if ferr == nil {
			for _, f := range favs {
				nums = append(nums, f.NodeNumber)
			}
		}
		if len(nums) > 0 {
			if current := s.statsCache.getMany(nums); len(current) > 0 {
				msg, _ := json.Marshal(map[string]any{
					"type":   "stats",
					"nodeID": nodeID,
					"stats":  current,
				})
				initial = append(initial, msg)
			}
		}
	}
	if links, ok := s.linksCache.get(nodeID); ok {
		msg, _ := json.Marshal(map[string]any{
			"type":   "links",
			"nodeID": nodeID,
			"links":  links,
		})
		initial = append(initial, msg)
	}

	s.sseBroker.StreamFrom(w, r, ch, initial...)
}

// handleAPINodeStats returns the current cached stats for all favorites of a node,
// plus the home node's own stats (for AllStarLink connectivity display).
func (s *Server) handleAPINodeStats(w http.ResponseWriter, r *http.Request) {
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
	favs, err := s.db.ListFavoritesByNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	nums := make([]string, 0, len(favs)+1)
	nums = append(nums, n.NodeNumber)
	for _, f := range favs {
		nums = append(nums, f.NodeNumber)
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
		Target    string `json:"target"`
		Mode      string `json:"mode"`      // see ilinkMap below
		Exclusive bool   `json:"exclusive"` // disconnect all others first
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Target == "" {
		http.Error(w, "body must be {\"target\":\"NNNNN\"}", http.StatusBadRequest)
		return
	}

	// ilink function codes from app_rpt.c
	ilinkMap := map[string]string{
		"transceive":         "3",
		"transceive_perm":    "13",
		"monitor":            "2",
		"monitor_perm":       "12",
		"monitor_local":      "8",
		"monitor_local_perm": "18",
	}
	ilink, ok := ilinkMap[body.Mode]
	if !ok {
		ilink = "3"
	}

	if body.Exclusive {
		// Only disconnect-all if there are actually active links; sending ilink 6
		// when nothing is connected can race with the subsequent ilink 3 and cancel it.
		if links, _ := s.linksCache.get(nodeID); len(links) > 0 {
			disconnCmd := fmt.Sprintf("rpt cmd %s ilink 6", n.NodeNumber)
			slog.Info("AMI disconnect-all (exclusive connect)", "node_id", nodeID, "cmd", disconnCmd)
			if _, err := s.amiMgr.SendActionWait(nodeID, map[string]string{
				"Action":  "Command",
				"Command": disconnCmd,
			}, 5*time.Second); err != nil {
				slog.Warn("AMI disconnect-all failed", "node_id", nodeID, "err", err)
			}
			// Give Asterisk time to finish the disconnect before queuing the connect.
			time.Sleep(500 * time.Millisecond)
		}
	}

	cmd := fmt.Sprintf("rpt cmd %s ilink %s %s", n.NodeNumber, ilink, body.Target)
	slog.Info("AMI connect", "node_id", nodeID, "cmd", cmd, "exclusive", body.Exclusive)
	resp, err := s.amiMgr.SendActionWait(nodeID, map[string]string{
		"Action":  "Command",
		"Command": cmd,
	}, 5*time.Second)
	if err != nil {
		slog.Warn("AMI connect failed", "node_id", nodeID, "cmd", cmd, "err", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	slog.Info("AMI connect response", "node_id", nodeID, "headers", resp.Headers)
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
		Target    string `json:"target"`
		Permanent bool   `json:"permanent"` // use ilink 11 (disconnect permanent link)
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck — empty body is valid (disconnect all)

	var cmd string
	if body.Target == "" {
		cmd = fmt.Sprintf("rpt cmd %s ilink 6", n.NodeNumber)
	} else if body.Permanent {
		cmd = fmt.Sprintf("rpt cmd %s ilink 11 %s", n.NodeNumber, body.Target)
	} else {
		cmd = fmt.Sprintf("rpt cmd %s ilink 1 %s", n.NodeNumber, body.Target)
	}

	slog.Info("AMI disconnect", "node_id", nodeID, "cmd", cmd)
	resp, err := s.amiMgr.SendActionWait(nodeID, map[string]string{
		"Action":  "Command",
		"Command": cmd,
	}, 5*time.Second)
	if err != nil {
		slog.Warn("AMI disconnect failed", "node_id", nodeID, "cmd", cmd, "err", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	slog.Info("AMI disconnect response", "node_id", nodeID, "headers", resp.Headers)
	writeJSON(w, map[string]any{"ok": true})
}

