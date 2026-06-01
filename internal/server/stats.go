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
	"allstar-yaamon/internal/db"
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

// loadFromDB pre-populates the cache from the stats_cache table so last-known
// values are available immediately after a server restart.
func (c *statsCache) loadFromDB(ctx context.Context, database *db.DB) error {
	rows, err := database.LoadStatsCache(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for num, raw := range rows {
		var s aslstats.NodeStats
		if json.Unmarshal(raw, &s) == nil {
			c.stats[num] = s
		}
	}
	slog.Info("stats cache: loaded from DB", "count", len(rows))
	return nil
}

// saveStatsCacheToDB persists successful fetch results to the stats_cache table.
// Error entries are skipped — only successful (non-error) results are stored.
func (s *Server) saveStatsCacheToDB(ctx context.Context, results map[string]aslstats.NodeStats) {
	entries := make(map[string]json.RawMessage, len(results))
	for num, st := range results {
		if st.Error != "" {
			continue
		}
		raw, err := json.Marshal(st)
		if err == nil {
			entries[num] = raw
		}
	}
	if err := s.db.SaveStatsCache(ctx, entries); err != nil {
		slog.Warn("stats cache: save to DB failed", "err", err)
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

// bulkHighMark and bulkLowMark define hysteresis thresholds for the adaptive
// fetch mode. When the number of stale nodes exceeds bulkHighMark the poller
// switches to the bulk endpoint (1 request/min/IP, returns all reporting nodes).
// It stays in bulk mode until the stale count drops to bulkLowMark or below,
// then reverts to individual rate-limited fetches (30 requests/min/IP total).
const (
	bulkHighMark    = 10
	bulkLowMark     = 4
	bulkMinInterval = 5 * time.Minute // multiple instances may share an IP; ASL allows 1 bulk req/min/IP
)

// pollStats is the background poller. It collects all unique node numbers
// across every active (SSE-subscribed) node in one pass, deduplicates them
// globally, checks which are stale, then calls fetchAdaptive once for the
// whole set. Results are stored in the shared cache and SSE events are
// published per node.
func (s *Server) pollStats(ctx context.Context) {
	nodes, err := s.db.ListNodes(ctx)
	if err != nil {
		return
	}

	type nodeSet struct {
		id   int64
		nums []string
	}

	globalSeen := make(map[string]struct{})
	sets := make([]nodeSet, 0, len(nodes))

	for _, n := range nodes {
		if !n.Enabled || !s.sseBroker.HasSubscribers(n.ID) {
			continue
		}
		favs, err := s.db.ListFavoritesByNode(ctx, n.ID)
		if err != nil {
			continue
		}
		localSeen := make(map[string]bool)
		nums := make([]string, 0, len(favs)+1)
		add := func(num string) {
			if !localSeen[num] {
				localSeen[num] = true
				nums = append(nums, num)
				globalSeen[num] = struct{}{}
			}
		}
		add(n.NodeNumber)
		for _, f := range favs {
			add(f.NodeNumber)
		}
		if linked, ok := s.linksCache.get(n.ID); ok {
			for num := range linked {
				add(num)
			}
		}
		sets = append(sets, nodeSet{id: n.ID, nums: nums})
	}

	if len(globalSeen) == 0 {
		return
	}

	// Identify which globally-unique node numbers have gone stale.
	stale := make([]string, 0, len(globalSeen))
	for num := range globalSeen {
		if st, ok := s.statsCache.get(num); !ok || st.Stale() {
			stale = append(stale, num)
		}
	}

	if len(stale) > 0 {
		results := s.fetchAdaptive(ctx, stale)
		// Store everything returned — bulk fetches include nodes beyond what
		// was explicitly requested, and we keep all of them.
		s.statsCache.update(results)
		s.saveStatsCacheToDB(ctx, results)
	}

	// Publish fresh cache snapshot to each node's SSE subscribers.
	for _, ns := range sets {
		s.publishCachedStats(ns.id, ns.nums)
	}
}

// fetchAdaptive chooses between the bulk endpoint and individual rate-limited
// fetches based on the number of stale node numbers, with hysteresis to avoid
// oscillating between modes.
//
// Bulk mode  (inBulkMode=true):  single request to /api/stats/, returns all
//
//	reporting nodes; the full result is kept in the cache.
//
// Individual mode (inBulkMode=false): per-node requests via FetchAll,
//
//	subject to the shared 28 req/min rate limiter.
func (s *Server) fetchAdaptive(ctx context.Context, stale []string) map[string]aslstats.NodeStats {
	s.adaptiveMu.Lock()
	n := len(stale)
	switch {
	case n > bulkHighMark:
		s.inBulkMode = true
	case n <= bulkLowMark:
		s.inBulkMode = false
	// else: in hysteresis band — keep current mode
	}
	useBulk := s.inBulkMode
	canBulk := time.Since(s.lastBulkAt) >= bulkMinInterval
	s.adaptiveMu.Unlock()

	slog.Debug("stats fetch", "stale", n, "bulk", useBulk, "can_bulk", canBulk)

	if useBulk && canBulk {
		all, err := s.fetcher.FetchBulk(ctx)
		if err == nil {
			s.adaptiveMu.Lock()
			s.lastBulkAt = time.Now()
			s.adaptiveMu.Unlock()
			return all
		}
		slog.Warn("stats bulk fetch failed, falling back to individual", "err", err)
	}
	return s.fetcher.FetchAll(ctx, stale)
}

// enrichFromAMI overrides connected_links with live AMI link counts for nodes
// that are home nodes managed by this instance. ASL cloud stats can lag or
// disagree with local AMI state; AMI is authoritative when available.
// linksCache is only populated while AMI is connected, so hasData=true implies
// the count came from a live poll — no separate IsConnected check is needed.
func (s *Server) enrichFromAMI(stats map[string]aslstats.NodeStats) {
	for num, st := range stats {
		v, ok := s.homeNodeNums.Load(num)
		if !ok {
			continue
		}
		links, hasData := s.linksCache.get(v.(int64))
		if !hasData {
			continue
		}
		st.ConnectedLinks = len(links)
		stats[num] = st
	}
}

// publishCachedStats reads nums from the stats cache and publishes one SSE
// event for nodeID containing whatever is currently cached.
func (s *Server) publishCachedStats(nodeID int64, nums []string) {
	subset := s.statsCache.getMany(nums)
	s.enrichFromAstdb(subset)
	s.enrichFromAMI(subset)
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

// enrichFromAstdb fills in Location (and Callsign when absent) for each entry
// in stats from the local AllStar node database. The stats API does not return
// location; astdb is the authoritative source for that field.
func (s *Server) enrichFromAstdb(stats map[string]aslstats.NodeStats) {
	for num, st := range stats {
		if st.Location != "" {
			continue
		}
		if n, ok := s.nodeDB.Lookup(num); ok {
			if st.Location == "" {
				st.Location = n.Location
			}
			if st.Callsign == "" {
				st.Callsign = n.Callsign
			}
			stats[num] = st
		}
	}
}

// pollNodeStats fetches fresh ASL stats for a single node on SSE connect and
// publishes the result. Only stale or missing entries are fetched; nodes with
// fresh cache are served immediately from cache. Uses FetchDirect (no rate
// limiter) so the initial page load is responsive.
func (s *Server) pollNodeStats(ctx context.Context, nodeID int64) {
	n, err := s.db.GetNodeByID(ctx, nodeID)
	if err != nil {
		return
	}
	favs, err := s.db.ListFavoritesByNode(ctx, nodeID)
	if err != nil {
		return
	}
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
	if linked, ok := s.linksCache.get(nodeID); ok {
		for num := range linked {
			add(num)
		}
	}

	// Only fetch what is stale or absent; in steady state (bulk cache warm)
	// this is usually empty and the function returns immediately from cache.
	stale := make([]string, 0, len(nums))
	for _, num := range nums {
		if st, ok := s.statsCache.get(num); !ok || st.Stale() {
			stale = append(stale, num)
		}
	}
	if len(stale) > 0 {
		results := s.fetcher.FetchDirect(ctx, stale, len(stale), 8)
		s.statsCache.update(results)
		s.saveStatsCacheToDB(ctx, results)
	}

	s.publishCachedStats(nodeID, nums)
}

// handleSSE streams live stats updates for a node to the browser.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "nodeID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}

	// SSE streams are intentionally long-lived. Remove the server-level write
	// deadline so it doesn't kill the connection while an ASL stats fetch is
	// in flight (fetching many favorites can take >30 s through the semaphore).
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}

	// Subscribe before triggering the fresh fetch so no published events are
	// dropped between the fetch completing and the stream starting.
	ch, cancel := s.sseBroker.Subscribe(nodeID)
	defer cancel()

	// Trigger an immediate fresh stats fetch for this node in the background.
	// Use WithoutCancel so the fetch and publish complete even if the browser
	// closes this SSE connection before the goroutine finishes (e.g. on refresh).
	go s.pollNodeStats(context.WithoutCancel(r.Context()), nodeID)

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
	result := s.statsCache.getMany(nums)
	s.enrichFromAstdb(result)
	writeJSON(w, result)
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

