package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
)

type connEntry struct {
	NodeNumber  string `json:"node_number"`
	Callsign    string `json:"callsign"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

type connResponse struct {
	NodeNumber      string      `json:"node_number"`
	Callsign        string      `json:"callsign"`
	LCnt            int         `json:"lcnt"`
	Connections     []connEntry `json:"connections"`
	BubbleChartURL  string      `json:"bubble_chart_url"`
	CacheAgeSeconds int         `json:"cache_age_seconds"`
}

// handleAPIConnections returns the connection list for a specific node number from the
// stats cache. id is the home-node DB ID (reserved for future direction info from
// linksCache). nodeNumber is the AllStar node number of the favorite being inspected.
// GET /api/nodes/{id}/connections/{nodeNumber}
func (s *Server) handleAPIConnections(w http.ResponseWriter, r *http.Request) {
	if _, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	nodeNumber := chi.URLParam(r, "nodeNumber")
	if nodeNumber == "" {
		http.Error(w, "node number required", http.StatusBadRequest)
		return
	}

	resp := connResponse{
		NodeNumber:      nodeNumber,
		Connections:     []connEntry{},
		BubbleChartURL:  "https://stats.allstarlink.org/getstatus.cgi?" + nodeNumber,
		CacheAgeSeconds: -1,
	}

	stats, ok := s.statsCache.get(nodeNumber)
	if !ok || stats.Error != "" {
		writeJSON(w, resp)
		return
	}

	resp.Callsign = stats.Callsign
	resp.LCnt = stats.ConnectedLinks
	resp.CacheAgeSeconds = int(time.Since(stats.FetchedAt).Seconds())

	// Fetch stats for connected nodes not in cache. Use FetchDirect (semaphore, no
	// rate-limit queue) with a hard timeout so the handler never hangs on large hubs.
	var missing []string
	for _, ln := range stats.LinkedNodes {
		if ls, ok2 := s.statsCache.get(ln); !ok2 || ls.Error != "" {
			missing = append(missing, ln)
		}
	}
	if len(missing) > 0 {
		fetchCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		fetched := s.fetcher.FetchDirect(fetchCtx, missing, 40, 10)
		s.statsCache.update(fetched)
	}

	for _, ln := range stats.LinkedNodes {
		e := connEntry{NodeNumber: ln}
		if ls, ok2 := s.statsCache.get(ln); ok2 && ls.Error == "" {
			e.Callsign = ls.Callsign
			e.Description = ls.Description
			e.Location = ls.Location
		}
		resp.Connections = append(resp.Connections, e)
	}

	writeJSON(w, resp)
}

// graphPageData is the template data for the full-page network graph.
type graphPageData struct {
	Username   string
	Permission string
	NodeNumber string
	HomeNodeID int64
}

// handleGraphPage serves the full-page D3 network graph for a given AllStar node number.
// GET /graph/{nodeNumber}
func (s *Server) handleGraphPage(w http.ResponseWriter, r *http.Request) {
	nodeNumber := chi.URLParam(r, "nodeNumber")
	if nodeNumber == "" {
		http.Error(w, "node number required", http.StatusBadRequest)
		return
	}

	nodes, err := s.db.ListNodes(r.Context())
	if err != nil || len(nodes) == 0 {
		http.Error(w, "no nodes configured", http.StatusNotFound)
		return
	}
	var homeNodeID int64
	for _, n := range nodes {
		if n.Enabled {
			homeNodeID = n.ID
			break
		}
	}
	if homeNodeID == 0 {
		homeNodeID = nodes[0].ID
	}

	sess := auth.FromContext(r.Context())
	data := graphPageData{NodeNumber: nodeNumber, HomeNodeID: homeNodeID}
	if sess != nil {
		data.Username = sess.Username
		data.Permission = sess.Permission
	}
	s.render(w, "graph", data)
}
