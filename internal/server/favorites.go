package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

type favoriteJSON struct {
	ID          int64  `json:"id"`
	NodeNumber  string `json:"node_number"`
	Callsign    string `json:"callsign"`
	Description string `json:"description"`
	Location    string `json:"location"`
	GroupName   string `json:"group_name"`
	SortOrder   int    `json:"sort_order"`
	Position    int    `json:"position"`
}

type favoriteInput struct {
	NodeNumber  string `json:"node_number"`
	Callsign    string `json:"callsign"`
	Description string `json:"description"`
	Location    string `json:"location"`
	GroupName   string `json:"group_name"`
	SortOrder   int    `json:"sort_order"`
}

func favToJSON(f db.Favorite) favoriteJSON {
	return favoriteJSON{
		ID:          f.ID,
		NodeNumber:  f.NodeNumber,
		Callsign:    f.Callsign,
		Description: f.Description,
		Location:    f.Location,
		GroupName:   f.GroupName,
		SortOrder:   f.SortOrder,
		Position:    f.Position,
	}
}

// enrichFromCache fills empty Callsign/Description/Location from the stats cache.
func (s *Server) enrichFromCache(num string, in *favoriteInput) {
	if in.Callsign != "" && in.Description != "" && in.Location != "" {
		return
	}
	st, ok := s.statsCache.get(num)
	if !ok {
		return
	}
	if in.Callsign == "" {
		in.Callsign = st.Callsign
	}
	if in.Description == "" {
		in.Description = st.Description
	}
	if in.Location == "" {
		in.Location = st.Location
	}
}

// handleFavoritesPage renders the favorites management settings page.
func (s *Server) handleFavoritesPage(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	data := dashboardData{}
	if sess != nil {
		data.Username = sess.Username
		data.Permission = sess.Permission
	}
	data.Nodes, _ = s.db.ListNodes(r.Context())

	if idStr := chi.URLParam(r, "nodeID"); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			for i := range data.Nodes {
				if data.Nodes[i].ID == id {
					n := data.Nodes[i]
					data.ActiveNode = &n
					break
				}
			}
		}
	}
	if data.ActiveNode == nil && len(data.Nodes) > 0 {
		n := data.Nodes[0]
		data.ActiveNode = &n
	}
	s.render(w, "favorites", data)
}

// handleAPIListFavorites returns all favorites for a node as JSON.
func (s *Server) handleAPIListFavorites(w http.ResponseWriter, r *http.Request) {
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
	result := make([]favoriteJSON, len(favs))
	for i, f := range favs {
		result[i] = favToJSON(f)
	}
	writeJSON(w, result)
}

// handleAPICreateFavorite adds a favorite to a node.
func (s *Server) handleAPICreateFavorite(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	var in favoriteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if in.NodeNumber == "" {
		http.Error(w, "node_number is required", http.StatusBadRequest)
		return
	}
	if in.GroupName == "" {
		in.GroupName = "default"
	}
	s.enrichFromCache(in.NodeNumber, &in)
	f, err := s.db.CreateFavorite(r.Context(), db.Favorite{
		NodeID:      nodeID,
		NodeNumber:  in.NodeNumber,
		Callsign:    in.Callsign,
		Description: in.Description,
		Location:    in.Location,
		GroupName:   in.GroupName,
		SortOrder:   in.SortOrder,
	})
	if err != nil {
		http.Error(w, "create favorite: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, favToJSON(*f))
}

// handleAPIUpdateFavorite updates a favorite's fields.
func (s *Server) handleAPIUpdateFavorite(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	favID, err := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	if err != nil {
		http.Error(w, "invalid favorite id", http.StatusBadRequest)
		return
	}
	var in favoriteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if in.NodeNumber == "" {
		http.Error(w, "node_number is required", http.StatusBadRequest)
		return
	}
	if in.GroupName == "" {
		in.GroupName = "default"
	}
	s.enrichFromCache(in.NodeNumber, &in)
	updated := db.Favorite{
		ID:          favID,
		NodeID:      nodeID,
		NodeNumber:  in.NodeNumber,
		Callsign:    in.Callsign,
		Description: in.Description,
		Location:    in.Location,
		GroupName:   in.GroupName,
		SortOrder:   in.SortOrder,
	}
	if err := s.db.UpdateFavorite(r.Context(), updated); err != nil {
		http.Error(w, "update favorite: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, favToJSON(updated))
}

// handleAPIReorderFavorites sets the display order of favorites for a node.
func (s *Server) handleAPIReorderFavorites(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	var body struct {
		Order []int64 `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Order) == 0 {
		http.Error(w, "body must be {\"order\":[id,...]}", http.StatusBadRequest)
		return
	}
	if err := s.db.ReorderFavorites(r.Context(), nodeID, body.Order); err != nil {
		http.Error(w, "reorder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAPICopyFavorites copies selected favorites from another node into this one.
func (s *Server) handleAPICopyFavorites(w http.ResponseWriter, r *http.Request) {
	nodeID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid node id", http.StatusBadRequest)
		return
	}
	var body struct {
		SourceNodeID int64   `json:"source_node_id"`
		IDs          []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.SourceNodeID == 0 || len(body.IDs) == 0 {
		http.Error(w, "source_node_id and ids are required", http.StatusBadRequest)
		return
	}
	n, err := s.db.CopyFavorites(r.Context(), body.SourceNodeID, nodeID, body.IDs)
	if err != nil {
		http.Error(w, "copy: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"copied": n})
}

// handleAPIDeleteFavorite removes a favorite.
func (s *Server) handleAPIDeleteFavorite(w http.ResponseWriter, r *http.Request) {
	favID, err := strconv.ParseInt(chi.URLParam(r, "fid"), 10, 64)
	if err != nil {
		http.Error(w, "invalid favorite id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteFavorite(r.Context(), favID); err != nil {
		http.Error(w, "delete favorite: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
