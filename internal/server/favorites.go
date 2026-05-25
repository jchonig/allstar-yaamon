package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

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
	}
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

// handleAPIUpdateFavorite updates a favorite's metadata or sort order.
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
	if in.GroupName == "" {
		in.GroupName = "default"
	}
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
