package server

import (
	"context"
	"encoding/json"
	"log/slog"

	"allstar-yaamon/internal/ami"
)

// startAMIEventListener subscribes to all AMI events from every managed node
// and reacts to link-state changes immediately, without waiting for the next
// poll tick.
func (s *Server) startAMIEventListener(ctx context.Context) {
	ch := s.amiMgr.Subscribe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ne, ok := <-ch:
				if !ok {
					return
				}
				s.handleAMIEvent(ctx, ne.NodeID, ne.Event)
			}
		}
	}()
}

// handleAMIEvent dispatches a single AMI event received from a managed node.
func (s *Server) handleAMIEvent(ctx context.Context, nodeID int64, evt ami.Event) {
	switch evt.Type() {
	case "NodeConn", "NodeDisconn":
		// A link connected or disconnected — re-poll immediately so SSE clients
		// get an up-to-date link list without waiting for the 5 s tick.
		slog.Debug("AMI link event", "node_id", nodeID, "event", evt.Type(),
			"channel", evt.Get("Channel"))
		s.pollNodeLinks(ctx, nodeID)

	case "FullyBooted":
		// Asterisk just finished booting — re-poll all links for this node.
		slog.Info("AMI FullyBooted", "node_id", nodeID)
		s.pollNodeLinks(ctx, nodeID)

	case "Hangup":
		// A channel hung up — could be a link drop; re-poll to confirm.
		s.pollNodeLinks(ctx, nodeID)

	case "DTMF", "DTMFBegin":
		// Push the digit to SSE subscribers so the dashboard can display it briefly.
		digit := evt.Get("Digit")
		if digit == "" {
			break
		}
		data, _ := json.Marshal(map[string]any{
			"type":    "dtmf",
			"nodeID":  nodeID,
			"digit":   digit,
			"channel": evt.Get("Channel"),
		})
		s.sseBroker.Publish(nodeID, data)
		slog.Debug("AMI DTMF", "node_id", nodeID, "digit", digit, "channel", evt.Get("Channel"))
	}
}
