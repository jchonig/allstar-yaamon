package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// linksCache stores the current link state of each home node as reported by
// "rpt show variables" via AMI. The value maps node number → connection type
// character (e.g. "T"=transceive, "M"=monitor, "L"=local, "P"=permanent).
type linksCache struct {
	mu    sync.RWMutex
	links map[int64]map[string]string // nodeID → {nodeNum: typeChar}
}

func newLinksCache() *linksCache {
	return &linksCache{links: make(map[int64]map[string]string)}
}

// get returns the cached link map and whether a poll result has ever been stored.
func (c *linksCache) get(nodeID int64) (links map[string]string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, exists := c.links[nodeID]
	return v, exists
}

// set stores the link map and returns true if the value changed (including first store).
func (c *linksCache) set(nodeID int64, linked map[string]string) bool {
	if linked == nil {
		linked = map[string]string{} // normalise: "no links" != "never polled"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	old, exists := c.links[nodeID]
	c.links[nodeID] = linked
	if !exists {
		return true // first poll — always publish
	}
	return !eqStringMaps(old, linked)
}

func eqStringMaps(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// startLinksPoller starts a background goroutine that polls AMI for link state
// every 5 seconds and pushes SSE updates when the connected set changes.
func (s *Server) startLinksPoller(ctx context.Context) {
	go func() {
		s.pollLinks(ctx)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.pollLinks(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Server) pollLinks(ctx context.Context) {
	nodes, err := s.db.ListNodes(ctx)
	if err != nil {
		return
	}
	for _, n := range nodes {
		if !n.Enabled || !s.amiMgr.IsConnected(n.ID) {
			continue
		}
		resp, err := s.amiMgr.SendActionWait(n.ID, map[string]string{
			"Action":  "Command",
			"Command": "rpt show variables " + n.NodeNumber,
		}, 5*time.Second)
		if err != nil {
			slog.Debug("links poll failed", "node_id", n.ID, "err", err)
			continue
		}
		linked := parseRPTALinks(resp.Get("Output"))
		slog.Debug("AMI links", "node_id", n.ID, "links", linked)
		if s.linksCache.set(n.ID, linked) {
			// Publish as {"nodeNum": "typeChar"} so the client knows connection type.
			data, _ := json.Marshal(map[string]any{
				"type":   "links",
				"nodeID": n.ID,
				"links":  linked,
			})
			s.sseBroker.Publish(n.ID, data)
		}
	}
}

// parseRPTALinks parses the RPT_ALINKS variable from "rpt show variables" output.
// RPT_ALINKS format: "<count>,<nodenum><typechar><keyedstate>[,...]"
// e.g. "1,41522TU" → {"41522":"T"}
// Known type chars: T=transceive, M=monitor, L=local monitor, P=permanent transceive.
// Known keyed states: U=unkeyed, K=keyed (not exposed in the returned map).
func parseRPTALinks(output string) map[string]string {
	const marker = "RPT_ALINKS="
	idx := strings.Index(output, marker)
	if idx < 0 {
		return nil
	}
	val := output[idx+len(marker):]
	if nl := strings.IndexAny(val, "\r\n"); nl >= 0 {
		val = val[:nl]
	}
	val = strings.TrimSpace(val)

	parts := strings.Split(val, ",")
	if len(parts) < 2 {
		return nil // "0" — no links
	}

	result := make(map[string]string, len(parts)-1)
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		// Format: <nodenum><typechar><keyedstate> — digits first, then type char.
		i := 0
		for i < len(p) && p[i] >= '0' && p[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(p) {
			continue
		}
		result[p[:i]] = string(p[i])
	}
	return result
}
