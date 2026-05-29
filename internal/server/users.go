package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

type userJSON struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Permission         string `json:"permission"`
	FullName           string `json:"full_name,omitempty"`
	AvatarURL          string `json:"avatar_url,omitempty"`
	TailscaleUsernames string `json:"tailscale_usernames,omitempty"`
}

type userInput struct {
	Username           string  `json:"username"`
	Password           string  `json:"password"`
	Permission         string  `json:"permission"`
	TailscaleUsernames *string `json:"tailscale_usernames"`
}

func userToJSON(u db.User) userJSON {
	return userJSON{
		ID:                 u.ID,
		Username:           u.Username,
		Permission:         u.Permission,
		FullName:           u.FullName,
		AvatarURL:          u.AvatarURL,
		TailscaleUsernames: u.TailscaleUsernames,
	}
}

// handleAPIListUsers returns all users (no password hash).
func (s *Server) handleAPIListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	result := make([]userJSON, len(users))
	for i, u := range users {
		result[i] = userToJSON(u)
	}
	writeJSON(w, result)
}

// handleAPICreateUser creates a new user.
// Admin can create non-superuser accounts; only superuser can create superusers.
func (s *Server) handleAPICreateUser(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	var in userInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if in.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if !db.ValidPermission(in.Permission) {
		http.Error(w, "invalid permission level", http.StatusBadRequest)
		return
	}
	if in.Permission == db.PermSuperuser && sess.Permission != db.PermSuperuser {
		http.Error(w, "only superusers can create superuser accounts", http.StatusForbidden)
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u, err := s.db.CreateUser(r.Context(), in.Username, hash, in.Permission)
	if err != nil {
		http.Error(w, "create user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, userToJSON(*u))
}

// handleAPIUpdateUser updates a user's permission and/or password.
func (s *Server) handleAPIUpdateUser(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	existing, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	// Non-superusers cannot touch superuser accounts.
	if existing.Permission == db.PermSuperuser && sess.Permission != db.PermSuperuser {
		http.Error(w, "only superusers can modify superuser accounts", http.StatusForbidden)
		return
	}

	var in userInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if in.Permission != "" {
		if !db.ValidPermission(in.Permission) {
			http.Error(w, "invalid permission level", http.StatusBadRequest)
			return
		}
		if in.Permission == db.PermSuperuser && sess.Permission != db.PermSuperuser {
			http.Error(w, "only superusers can grant superuser permission", http.StatusForbidden)
			return
		}
		if err := s.db.UpdateUserPermission(r.Context(), id, in.Permission); err != nil {
			http.Error(w, "update permission: "+err.Error(), http.StatusInternalServerError)
			return
		}
		existing.Permission = in.Permission
	}

	if in.Password != "" {
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.db.UpdateUserPassword(r.Context(), id, hash); err != nil {
			http.Error(w, "update password: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if in.TailscaleUsernames != nil {
		if err := s.db.UpdateUserTailscaleUsernames(r.Context(), id, *in.TailscaleUsernames); err != nil {
			http.Error(w, "update tailscale usernames: "+err.Error(), http.StatusInternalServerError)
			return
		}
		existing.TailscaleUsernames = *in.TailscaleUsernames
	}

	writeJSON(w, userToJSON(*existing))
}

// handleAPIDeleteUser deletes a user. Only superusers can delete; last superuser is protected.
func (s *Server) handleAPIDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	existing, err := s.db.GetUserByID(r.Context(), id)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if existing.Permission == db.PermSuperuser {
		n, _ := s.db.CountSuperusers(r.Context())
		if n <= 1 {
			http.Error(w, "cannot delete the last superuser", http.StatusConflict)
			return
		}
	}
	if err := s.db.DeleteUser(r.Context(), id); err != nil {
		http.Error(w, "delete user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUsersPage renders the user management page.
func (s *Server) handleUsersPage(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	data := struct{ pageData }{pageData: s.newPageData()}
	fillSession(&data.pageData, sess)
	s.render(w, "users", data)
}
