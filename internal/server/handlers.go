package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
	"allstar-yaamon/internal/version"
)

// pageData contains fields common to all authenticated page templates.
type pageData struct {
	Username   string
	Permission string
	FullName   string
	AvatarURL  string
	Version    string
	RepoURL    string
}

func (s *Server) newPageData() pageData {
	return pageData{
		Version: version.Version,
		RepoURL: s.cfg.UI.FooterURL,
	}
}

// fillSession copies the authenticated session fields into pd.
func fillSession(pd *pageData, sess *auth.Session) {
	if sess == nil {
		return
	}
	pd.Username = sess.Username
	pd.Permission = sess.Permission
	pd.FullName = sess.FullName
	pd.AvatarURL = sess.AvatarURL
}

// handleHealth returns JSON {"status":"ok"} for Docker HEALTHCHECK and integration tests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Setup (first-run admin creation) ---

type setupData struct {
	pageData // always zero — no user logged in during setup
	Error    string
}

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

	if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission, user.FullName, user.AvatarURL); err != nil {
		s.render(w, "setup", setupData{Error: "Session error"})
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// --- Login / Logout ---

type loginData struct {
	pageData // always zero — no user logged in yet
	Error    string
}

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
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}

	if !s.loginLimiter.Allow(ip) {
		http.Error(w, "Too many failed login attempts — try again in a minute", http.StatusTooManyRequests)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.render(w, "login", loginData{Error: "Invalid form data"})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := s.db.GetUser(r.Context(), username)
	if err != nil {
		s.loginLimiter.RecordFailure(ip)
		s.renderLoginError(w, r)
		return
	}
	if err := auth.CheckPassword(user.Password, password); err != nil {
		s.loginLimiter.RecordFailure(ip)
		s.renderLoginError(w, r)
		return
	}

	s.loginLimiter.RecordSuccess(ip)
	if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission, user.FullName, user.AvatarURL); err != nil {
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

const lastDashboardCookie = "yaamon_last_dashboard"

// handleDashboardOverview sets the last-dashboard cookie to "overview" and
// redirects to /dashboard, which will then show the summary page.
func (s *Server) handleDashboardOverview(w http.ResponseWriter, r *http.Request) {
	setLastDashboard(w, "overview")
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func setLastDashboard(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     lastDashboardCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   30 * 24 * 3600,
		SameSite: http.SameSiteLaxMode,
	})
}

// --- Dashboard ---

type dashboardData struct {
	pageData
	Nodes      []db.Node
	ActiveNode *db.Node
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	data := dashboardData{pageData: s.newPageData()}
	fillSession(&data.pageData, sess)
	data.Nodes, _ = s.db.ListNodes(r.Context())
	s.fillNodeInfo(data.Nodes)

	// Determine active node from URL param or default to first.
	if idStr := chi.URLParam(r, "nodeID"); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			for i := range data.Nodes {
				if data.Nodes[i].ID == id {
					n := data.Nodes[i]
					data.ActiveNode = &n
					setLastDashboard(w, idStr)
					break
				}
			}
		}
	}
	if data.ActiveNode == nil {
		if len(data.Nodes) == 1 {
			// Single-node install: go straight to that node's dashboard.
			http.Redirect(w, r, fmt.Sprintf("/dashboard/%d", data.Nodes[0].ID), http.StatusFound)
			return
		}
		if len(data.Nodes) > 1 {
			// Multi-node: check cookie for last visited page.
			if c, err := r.Cookie(lastDashboardCookie); err == nil && c.Value != "overview" {
				if id, err := strconv.ParseInt(c.Value, 10, 64); err == nil {
					for i := range data.Nodes {
						if data.Nodes[i].ID == id {
							http.Redirect(w, r, fmt.Sprintf("/dashboard/%d", id), http.StatusFound)
							return
						}
					}
				}
			}
			// No cookie, invalid cookie, or "overview" — show summary and record it.
			setLastDashboard(w, "overview")
		}
		// 0 nodes: show empty state (ActiveNode stays nil, no cookie set).
	}

	s.render(w, "dashboard", data)
}

// fillNodeInfo fills in Description and Location from the astdb for any node
// that has those fields empty.
func (s *Server) fillNodeInfo(nodes []db.Node) {
	for i := range nodes {
		if nodes[i].Description == "" || nodes[i].Location == "" {
			if entry, ok := s.nodeDB.Lookup(nodes[i].NodeNumber); ok {
				if nodes[i].Description == "" {
					nodes[i].Description = entry.Description
				}
				if nodes[i].Location == "" {
					nodes[i].Location = entry.Location
				}
			}
		}
	}
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
