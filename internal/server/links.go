package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LinkState holds the real-time state of one connected node as reported by
// AMI's "rpt show variables" output.
type LinkState struct {
	Type        string    `json:"type"`         // T=transceive M=monitor L=local P=permanent
	Keyed       bool      `json:"keyed"`        // remote node is currently transmitting to us
	ConnectedAt time.Time `json:"connected_at"` // when the link was first seen
}

// linksCache stores the current link state of each home node as reported by
// "rpt show variables" via AMI.
type linksCache struct {
	mu    sync.RWMutex
	links map[int64]map[string]LinkState // nodeID → {nodeNum: LinkState}
}

func newLinksCache() *linksCache {
	return &linksCache{links: make(map[int64]map[string]LinkState)}
}

// get returns the cached link map and whether a poll result has ever been stored.
func (c *linksCache) get(nodeID int64) (links map[string]LinkState, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, exists := c.links[nodeID]
	return v, exists
}

// set stores the link map, preserving ConnectedAt for links that were already
// present. Returns true if the visible state changed (including first store).
func (c *linksCache) set(nodeID int64, linked map[string]LinkState) bool {
	if linked == nil {
		linked = map[string]LinkState{}
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	old, exists := c.links[nodeID]
	// Preserve ConnectedAt for existing links; stamp new ones with now.
	for num, ls := range linked {
		if prev, had := old[num]; had && !prev.ConnectedAt.IsZero() {
			ls.ConnectedAt = prev.ConnectedAt
		} else {
			ls.ConnectedAt = now
		}
		linked[num] = ls
	}
	c.links[nodeID] = linked
	if !exists {
		return true
	}
	return !eqLinkMaps(old, linked)
}

// eqLinkMaps compares two link maps, ignoring ConnectedAt (timestamp changes
// should not trigger a re-render).
func eqLinkMaps(a, b map[string]LinkState) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va.Type != vb.Type || va.Keyed != vb.Keyed {
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
		s.homeNodeNums.Store(n.NodeNumber, n.ID)
		if !n.Enabled || !s.amiMgr.IsConnected(n.ID) {
			continue
		}
		s.pollNodeLinksInner(ctx, n.ID, n.NodeNumber)
	}
}

// pollNodeLinks re-polls link state for a single node and publishes an SSE
// update if anything changed. Used by the AMI event listener for immediate
// reaction to NodeConn/NodeDisconn/Hangup events.
func (s *Server) pollNodeLinks(ctx context.Context, nodeID int64) {
	n, err := s.db.GetNodeByID(ctx, nodeID)
	if err != nil || !n.Enabled || !s.amiMgr.IsConnected(nodeID) {
		return
	}
	s.pollNodeLinksInner(ctx, nodeID, n.NodeNumber)
}

// pollNodeLinksInner runs the AMI command, parses the result, and publishes SSE
// when anything changed. Shared by the poller and the event-triggered path.
func (s *Server) pollNodeLinksInner(ctx context.Context, nodeID int64, nodeNumber string) {
	resp, err := s.amiMgr.SendActionWait(nodeID, map[string]string{
		"Action":  "Command",
		"Command": "rpt show variables " + nodeNumber,
	}, 5*time.Second)
	if err != nil {
		slog.Debug("links poll failed", "node_id", nodeID, "err", err)
		return
	}
	output := resp.Get("Output")
	linked := parseRPTALinks(output)
	txKeyed := parseRPTTXKeyed(output)
	slog.Debug("AMI links", "node_id", nodeID, "links", linked, "tx_keyed", txKeyed)
	if s.linksCache.set(nodeID, linked) {
		data, _ := json.Marshal(map[string]any{
			"type":     "links",
			"nodeID":   nodeID,
			"links":    linked,
			"tx_keyed": txKeyed,
		})
		s.sseBroker.Publish(nodeID, data)
	}
}

// parseRPTALinks parses the RPT_ALINKS variable from "rpt show variables" output.
// RPT_ALINKS format: "<count>,<nodenum><typechar><keyedstate>[,...]"
// e.g. "1,41522TK" → {"41522": {Type:"T", Keyed:true}}
// Type chars: T=transceive, M=monitor, L=local monitor, P=permanent transceive.
// Keyed state: K=keyed/transmitting, U=unkeyed/idle.
func parseRPTALinks(output string) map[string]LinkState {
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

	result := make(map[string]LinkState, len(parts)-1)
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		// Format: <nodeident><typechar><keyedstate>
		// nodeident may be a numeric node number (e.g. "667342") or a callsign
		// (e.g. "KR4YXX" for a direct/IAXRPT client). The last char is always
		// the keyed state (K or U) and the second-to-last is the type char.
		if len(p) < 3 {
			continue
		}
		nodeNum := p[:len(p)-2]
		result[nodeNum] = LinkState{
			Type:  string(p[len(p)-2]),
			Keyed: p[len(p)-1] == 'K',
		}
	}
	return result
}

// parseRPTTXKeyed extracts the RPT_TXKEYED flag from "rpt show variables" output.
// Returns true when the home node's transmitter is currently keyed.
func parseRPTTXKeyed(output string) bool {
	const marker = "RPT_TXKEYED="
	idx := strings.Index(output, marker)
	if idx < 0 {
		return false
	}
	val := output[idx+len(marker):]
	if nl := strings.IndexAny(val, "\r\n"); nl >= 0 {
		val = val[:nl]
	}
	return strings.TrimSpace(val) == "1"
}
