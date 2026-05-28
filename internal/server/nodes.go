package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/ami"
	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

// nodeJSON is the JSON representation of a node with its live AMI status.
type nodeJSON struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	NodeNumber  string `json:"node_number"`
	AMIHost     string `json:"ami_host"`
	AMIPort     int    `json:"ami_port"`
	AMIUser     string `json:"ami_user"`
	Enabled     bool   `json:"enabled"`
	Connected   bool   `json:"connected"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

// nodeInput is the shape of node create/update request bodies (AMIPass not in nodeJSON).
type nodeInput struct {
	Name        string `json:"name"`
	NodeNumber  string `json:"node_number"`
	AMIHost     string `json:"ami_host"`
	AMIPort     int    `json:"ami_port"`
	AMIUser     string `json:"ami_user"`
	AMIPass     string `json:"ami_pass"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

func (s *Server) nodeToJSON(n db.Node) nodeJSON {
	return nodeJSON{
		ID:          n.ID,
		Name:        n.Name,
		NodeNumber:  n.NodeNumber,
		AMIHost:     n.AMIHost,
		AMIPort:     n.AMIPort,
		AMIUser:     n.AMIUser,
		Enabled:     n.Enabled,
		Connected:   s.amiMgr.IsConnected(n.ID),
		Description: n.Description,
		Location:    n.Location,
	}
}

// handleAPIListNodes returns all nodes as JSON.
func (s *Server) handleAPIListNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.db.ListNodes(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	result := make([]nodeJSON, len(nodes))
	for i, n := range nodes {
		result[i] = s.nodeToJSON(n)
	}
	writeJSON(w, result)
}

// handleAPICreateNode creates a new node and starts its AMI client.
func (s *Server) handleAPICreateNode(w http.ResponseWriter, r *http.Request) {
	var in nodeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if in.Name == "" || in.NodeNumber == "" {
		http.Error(w, "name and node_number are required", http.StatusBadRequest)
		return
	}
	if !validNodeNumber(in.NodeNumber) {
		http.Error(w, "node_number must be 4–10 digits", http.StatusBadRequest)
		return
	}
	if in.AMIUser == "" {
		in.AMIUser = "admin"
	}
	if in.AMIPort == 0 {
		in.AMIPort = 5038
	}
	if in.AMIHost == "" {
		in.AMIHost = "localhost"
	}

	n, err := s.db.CreateNode(r.Context(), db.Node{
		Name:        in.Name,
		NodeNumber:  in.NodeNumber,
		AMIHost:     in.AMIHost,
		AMIPort:     in.AMIPort,
		AMIUser:     in.AMIUser,
		AMIPass:     in.AMIPass,
		Enabled:     in.Enabled,
		Description: in.Description,
		Location:    in.Location,
	})
	if err != nil {
		http.Error(w, "create node: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.amiMgr.Add(*n)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, s.nodeToJSON(*n))
}

// handleAPIUpdateNode updates an existing node and restarts its AMI client.
func (s *Server) handleAPIUpdateNode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	existing, err := s.db.GetNodeByID(r.Context(), id)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	var in nodeInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Preserve existing values for omitted fields.
	if in.Name == "" {
		in.Name = existing.Name
	}
	if in.NodeNumber == "" {
		in.NodeNumber = existing.NodeNumber
	}
	if in.NodeNumber != existing.NodeNumber && !validNodeNumber(in.NodeNumber) {
		http.Error(w, "node_number must be 4–10 digits", http.StatusBadRequest)
		return
	}
	if in.AMIHost == "" {
		in.AMIHost = existing.AMIHost
	}
	if in.AMIPort == 0 {
		in.AMIPort = existing.AMIPort
	}
	if in.AMIUser == "" {
		in.AMIUser = existing.AMIUser
	}
	amiPass := in.AMIPass
	if amiPass == "" {
		amiPass = existing.AMIPass
	}

	updated := db.Node{
		ID:          id,
		Name:        in.Name,
		NodeNumber:  in.NodeNumber,
		AMIHost:     in.AMIHost,
		AMIPort:     in.AMIPort,
		AMIUser:     in.AMIUser,
		AMIPass:     amiPass,
		Enabled:     in.Enabled,
		Description: in.Description,
		Location:    in.Location,
	}
	if err := s.db.UpdateNode(r.Context(), updated); err != nil {
		http.Error(w, "update node: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.amiMgr.Add(updated)
	writeJSON(w, s.nodeToJSON(updated))
}

// handleAPINodeSecret returns the stored AMI password for a node.
// Only accessible to admin+ users so the nodes page can pre-fill the edit form.
func (s *Server) handleAPINodeSecret(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	n, err := s.db.GetNodeByID(r.Context(), id)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"secret": n.AMIPass})
}

// handleAPIDeleteNode deletes a node and removes its AMI client.
func (s *Server) handleAPIDeleteNode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	s.amiMgr.Remove(id)

	if err := s.db.DeleteNode(r.Context(), id); err != nil {
		http.Error(w, "delete node: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAPITestNode dials AMI for a node and returns {"ok":true} or {"ok":false,"error":"..."}.
func (s *Server) handleAPITestNode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	n, err := s.db.GetNodeByID(r.Context(), id)
	if err != nil {
		http.Error(w, "node not found", http.StatusNotFound)
		return
	}

	testErr := ami.TestConnection(n.AMIHost, n.AMIPort, n.AMIUser, n.AMIPass)
	result := map[string]any{"ok": testErr == nil}
	if testErr != nil {
		result["error"] = testErr.Error()
	}
	writeJSON(w, result)
}

// handleNodesPage renders the node management page.
func (s *Server) handleNodesPage(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	data := struct{ pageData }{pageData: s.newPageData()}
	fillSession(&data.pageData, sess)
	s.render(w, "nodes", data)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
