package server

import (
	"encoding/json"
	"net/http"
)

type loginData struct {
	Error string
}

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login", loginData{})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	// Phase 2 will implement real authentication.
	// For now, redirect back with an error so the form is exercisable.
	s.render(w, "login", loginData{Error: "Authentication not yet configured — Phase 2"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	// Phase 2 will add auth check; Phase 4 will add node/favorites data.
	s.render(w, "dashboard", nil)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
