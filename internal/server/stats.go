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
		// Always include the home node's own number so its AllStarLink
		// connectivity status is available on the dashboard and summary page.
		nums := make([]string, 0, len(favs)+1)
		nums = append(nums, n.NodeNumber)
		allNums[n.NodeNumber] = struct{}{}
		for _, f := range favs {
			nums = append(nums, f.NodeNumber)
			allNums[f.NodeNumber] = struct{}{}
		}
		work = append(work, nodeWork{id: n.ID, nums: nums})
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
	// Include the home node's own number so its AllStarLink status is visible.
	var statsMsg []byte
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
			current := s.statsCache.getMany(nums)
			if len(current) > 0 {
				statsMsg, _ = json.Marshal(map[string]any{
					"type":   "stats",
					"nodeID": nodeID,
					"stats":  current,
				})
			}
		}
	}

	// Send cached link state immediately as the second event (if already polled).
	var linksMsg []byte
	if links, ok := s.linksCache.get(nodeID); ok {
		linksMsg, _ = json.Marshal(map[string]any{
			"type":   "links",
			"nodeID": nodeID,
			"links":  links,
		})
	}

	s.sseBroker.Stream(w, r, nodeID, statsMsg, linksMsg)
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

