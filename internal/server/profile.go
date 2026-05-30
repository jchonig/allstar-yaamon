package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
)

const avatarMaxBytes = 2 << 20 // 2 MB

func avatarConfigKey(userID int64) string {
	return "user_avatar_" + strconv.FormatInt(userID, 10)
}

// effectiveAvatarURL returns the URL to display for a user's avatar:
// the uploaded-data endpoint if data is stored, else the external avatar_url.
func (s *Server) effectiveAvatarURL(r *http.Request, userID int64, externalURL string) string {
	data, _ := s.db.GetConfig(r.Context(), avatarConfigKey(userID))
	if data != "" {
		return fmt.Sprintf("/api/users/%d/avatar", userID)
	}
	return externalURL
}

// validateSessionUser verifies the session user still exists in the database.
// Proxy-auth sessions (AuthMethod != "") are stateless and don't need DB validation.
// Deleted accounts are cleared and the request is redirected/rejected.
func (s *Server) validateSessionUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sess := auth.FromContext(r.Context()); sess != nil && sess.AuthMethod == "" {
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

// handleAPIGetProfile returns the current user's profile fields.
// GET /api/profile
func (s *Server) handleAPIGetProfile(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	if sess == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Include the Tailscale login detected in this request (if any) so the UI
	// can offer a one-click "add" when the header is present but not yet mapped.
	tailscaleLogin := strings.TrimSpace(r.Header.Get(s.cfg.TailscaleAuth.UserHeader))

	writeJSON(w, map[string]any{
		"username":            user.Username,
		"full_name":           user.FullName,
		"avatar_url":          s.effectiveAvatarURL(r, user.ID, user.AvatarURL),
		"tailscale_usernames": user.TailscaleUsernames,
		"tailscale_login":     tailscaleLogin,
	})
}

// handleAPIUpdateProfile updates the current user's full name, optionally an external avatar
// URL (which clears any uploaded avatar), and optionally the password.
// PUT /api/profile
// Body: {full_name, avatar_url (optional external URL), current_password, new_password}
func (s *Server) handleAPIUpdateProfile(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	if sess == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	var body struct {
		FullName           string  `json:"full_name"`
		AvatarURL          string  `json:"avatar_url"`       // external URL; clears uploaded avatar if non-empty
		ClearAvatar        bool    `json:"clear_avatar"`     // true when explicitly clearing avatar URL
		CurrentPassword    string  `json:"current_password"`
		NewPassword        string  `json:"new_password"`
		TailscaleUsernames *string `json:"tailscale_usernames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Local password change is disallowed for proxy-auth accounts ("*" sentinel).
	if body.NewPassword != "" {
		if user.Password == "*" {
			http.Error(w, "password change not available for proxy-authenticated accounts", http.StatusForbidden)
			return
		}
		if body.CurrentPassword == "" {
			http.Error(w, "current password is required to set a new password", http.StatusBadRequest)
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

	if body.TailscaleUsernames != nil {
		if err := s.db.UpdateUserTailscaleUsernames(r.Context(), sess.UserID, *body.TailscaleUsernames); err != nil {
			http.Error(w, "update tailscale usernames: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Determine the new avatar_url to store in the DB.
	newAvatarURL := user.AvatarURL // default: keep existing
	if body.AvatarURL != "" {
		// External URL provided — clear any uploaded avatar data so URL takes effect.
		_ = s.db.SetConfig(r.Context(), avatarConfigKey(sess.UserID), "")
		newAvatarURL = body.AvatarURL
	} else if body.ClearAvatar {
		_ = s.db.SetConfig(r.Context(), avatarConfigKey(sess.UserID), "")
		newAvatarURL = ""
	}

	if err := s.db.UpdateUserProfile(r.Context(), sess.UserID, body.FullName, newAvatarURL); err != nil {
		http.Error(w, "update profile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user, err = s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}

	// Proxy-auth sessions are stateless; no cookie to refresh.
	if sess.AuthMethod == "" {
		avatarURL := s.effectiveAvatarURL(r, user.ID, user.AvatarURL)
		if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission, user.FullName, avatarURL); err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true, "full_name": user.FullName})
}

// handleAPIUploadAvatar stores an uploaded image as the current user's avatar.
// POST /api/profile/avatar — raw image bytes in body, Content-Type must be an image type.
func (s *Server) handleAPIUploadAvatar(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	if sess == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, avatarMaxBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(data) < 4 {
		http.Error(w, "file too small", http.StatusBadRequest)
		return
	}
	if !isPNG(data) && !isICO(data) && !isJPEG(data) && !isGIF(data) && !isWEBP(data) {
		http.Error(w, "unsupported image format (PNG, JPEG, GIF, or WebP required)", http.StatusBadRequest)
		return
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	if err := s.db.SetConfig(r.Context(), avatarConfigKey(sess.UserID), encoded); err != nil {
		http.Error(w, "store failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Point the user's avatar_url at the serving endpoint so the navbar reflects it.
	apiPath := fmt.Sprintf("/api/users/%d/avatar", sess.UserID)
	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}
	if err := s.db.UpdateUserProfile(r.Context(), sess.UserID, user.FullName, apiPath); err != nil {
		http.Error(w, "update profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if sess.AuthMethod == "" {
		if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission, user.FullName, apiPath); err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true, "avatar_url": apiPath})
}

// handleAPIDeleteAvatar removes the current user's uploaded avatar.
// DELETE /api/profile/avatar
func (s *Server) handleAPIDeleteAvatar(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	if sess == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	if err := s.db.SetConfig(r.Context(), avatarConfigKey(sess.UserID), ""); err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}
	if err := s.db.UpdateUserProfile(r.Context(), sess.UserID, user.FullName, ""); err != nil {
		http.Error(w, "update profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if sess.AuthMethod == "" {
		if err := s.sessions.SetSession(w, user.ID, user.Username, user.Permission, user.FullName, ""); err != nil {
			http.Error(w, "session error", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIGetAvatar serves the uploaded avatar image for a user.
// GET /api/users/{id}/avatar
func (s *Server) handleAPIGetAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	raw, err := s.db.GetConfig(r.Context(), avatarConfigKey(id))
	if err != nil || raw == "" {
		http.NotFound(w, r)
		return
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ct := "image/png"
	if isJPEG(data) {
		ct = "image/jpeg"
	} else if isGIF(data) {
		ct = "image/gif"
	} else if isWEBP(data) {
		ct = "image/webp"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(data) //nolint:errcheck
}
