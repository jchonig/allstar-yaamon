package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"allstar-yaamon/internal/auth"
)

// validateSessionUser verifies the session user still exists in the database.
// Deleted accounts are cleared and the request is redirected/rejected.
func (s *Server) validateSessionUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess := auth.FromContext(r.Context()); sess != nil {
			if _, err := s.db.GetUserByID(r.Context(), sess.UserID); err != nil {
				s.sessions.ClearSession(w)
				if strings.HasPrefix(r.URL.Path, "/api/") {
					http.Error(w, "session expired", http.StatusUnauthorized)
					return
				}
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// handleAPIUpdateProfile updates the current user's full name, avatar URL, and optionally password.
// PUT /api/profile
// Body: {full_name, avatar_url, current_password (required if new_password set), new_password}
func (s *Server) handleAPIUpdateProfile(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	if sess == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	var body struct {
		FullName        string `json:"full_name"`
		AvatarURL       string `json:"avatar_url"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if body.NewPassword != "" {
		if body.CurrentPassword == "" {
			http.Error(w, "current password is required to set a new password", http.StatusBadRequest)
			return
		}
		user, err := s.db.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := auth.CheckPassword(user.Password, body.CurrentPassword); err != nil {
			http.Error(w, "current password is incorrect", http.StatusUnauthorized)
			return
		}
		hash, err := auth.HashPassword(body.NewPassword)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.db.UpdateUserPassword(r.Context(), sess.UserID, hash); err != nil {
			http.Error(w, "update password: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := s.db.UpdateUserProfile(r.Context(), sess.UserID, body.FullName, body.AvatarURL); err != nil {
		http.Error(w, "update profile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}

	if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission, user.FullName, user.AvatarURL); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"ok":         true,
		"full_name":  user.FullName,
		"avatar_url": user.AvatarURL,
		"username":   user.Username,
		"permission": user.Permission,
	})
}
