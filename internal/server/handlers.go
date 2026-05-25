package server

import (
	"encoding/json"
	"net/http"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

// handleHealth returns JSON {"status":"ok"} for Docker HEALTHCHECK and integration tests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Setup (first-run admin creation) ---

type setupData struct{ Error string }

func (s *Server) handleSetupGet(w http.ResponseWriter, r *http.Request) {
	// If users exist, setup is complete — redirect to login.
	n, err := s.db.CountUsers(r.Context())
	if err != nil || n > 0 {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	s.render(w, "setup", setupData{})
}

func (s *Server) handleSetupPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	n, err := s.db.CountUsers(ctx)
	if err != nil || n > 0 {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.render(w, "setup", setupData{Error: "Invalid form data"})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if username == "" {
		s.render(w, "setup", setupData{Error: "Username is required"})
		return
	}
	if password != confirm {
		s.render(w, "setup", setupData{Error: "Passwords do not match"})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		s.render(w, "setup", setupData{Error: err.Error()})
		return
	}

	user, err := s.db.CreateUser(ctx, username, hash, db.PermSuperuser)
	if err != nil {
		s.render(w, "setup", setupData{Error: "Failed to create user: " + err.Error()})
		return
	}

	if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission); err != nil {
		s.render(w, "setup", setupData{Error: "Session error"})
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// --- Login / Logout ---

type loginData struct{ Error string }

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	if sess := auth.FromContext(r.Context()); sess != nil {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}
	// If no users exist, redirect to setup.
	n, _ := s.db.CountUsers(r.Context())
	if n == 0 {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	s.render(w, "login", loginData{})
}

func (s *Server) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, "login", loginData{Error: "Invalid form data"})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUser(r.Context(), username)
	if err != nil {
		s.renderLoginError(w, r)
		return
	}
	if err := auth.CheckPassword(user.Password, password); err != nil {
		s.renderLoginError(w, r)
		return
	}

	if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	next := r.FormValue("next")
	if next == "" || next[0] != '/' {
		next = "/dashboard"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (s *Server) renderLoginError(w http.ResponseWriter, r *http.Request) {
	// Same error for unknown user and wrong password — don't leak which one.
	w.WriteHeader(http.StatusUnauthorized)
	s.render(w, "login", loginData{Error: "Invalid username or password"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.sessions.ClearSession(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// --- Dashboard ---

type dashboardData struct {
	Username   string
	Permission string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	data := dashboardData{}
	if sess != nil {
		data.Username = sess.Username
		data.Permission = sess.Permission
	}
	s.render(w, "dashboard", data)
}

// setupGuard redirects every request to /setup when no users exist,
// except for /setup itself and static assets.
func (s *Server) setupGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setup" || r.URL.Path == "/health" ||
			len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/static/" {
			next.ServeHTTP(w, r)
			return
		}
		n, err := s.db.CountUsers(r.Context())
		if err == nil && n == 0 {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
