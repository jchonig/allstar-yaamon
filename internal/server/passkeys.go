package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	wawebauthn "github.com/go-webauthn/webauthn/webauthn"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

const waCookieName = "yaamon_wa_session"
const waCookieTTL = 5 * time.Minute

// waUser wraps db.User + its credentials to implement webauthn.User.
type waUser struct {
	user  *db.User
	creds []db.Credential
}

func (u *waUser) WebAuthnID() []byte { return u.user.WebAuthnID }
func (u *waUser) WebAuthnName() string { return u.user.Username }
func (u *waUser) WebAuthnDisplayName() string {
	if u.user.FullName != "" {
		return u.user.FullName
	}
	return u.user.Username
}
func (u *waUser) WebAuthnCredentials() []wawebauthn.Credential {
	out := make([]wawebauthn.Credential, 0, len(u.creds))
	for _, c := range u.creds {
		var cred wawebauthn.Credential
		if err := json.Unmarshal([]byte(c.CredentialJSON), &cred); err == nil {
			out = append(out, cred)
		}
	}
	return out
}

// --- helpers ---

func (s *Server) buildWAUser(r *http.Request, userID int64) (*waUser, error) {
	ctx := r.Context()
	u, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	u.WebAuthnID, err = s.db.GetOrSetWebAuthnID(ctx, userID)
	if err != nil {
		return nil, err
	}
	creds, err := s.db.ListCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &waUser{user: u, creds: creds}, nil
}

func (s *Server) storeWASession(w http.ResponseWriter, r *http.Request, ceremony string, userID *int64, sd *wawebauthn.SessionData) error {
	b, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	sessionID, err := auth.GenerateSecret()
	if err != nil {
		return err
	}
	exp := time.Now().Add(waCookieTTL)
	if err := s.db.CreateWebAuthnSession(r.Context(), sessionID, ceremony, userID, string(b), exp); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     waCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(waCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.sessions.IsSecure(),
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (s *Server) fetchWASession(r *http.Request, ceremony string) (*wawebauthn.SessionData, error) {
	c, err := r.Cookie(waCookieName)
	if err != nil {
		return nil, fmt.Errorf("no session cookie")
	}
	row, err := s.db.GetAndDeleteWebAuthnSession(r.Context(), c.Value)
	if err != nil {
		return nil, fmt.Errorf("session not found or expired")
	}
	if row.Ceremony != ceremony {
		return nil, fmt.Errorf("session ceremony mismatch")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, fmt.Errorf("session expired")
	}
	var sd wawebauthn.SessionData
	if err := json.Unmarshal([]byte(row.SessionJSON), &sd); err != nil {
		return nil, err
	}
	return &sd, nil
}

func clearWACookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: waCookieName, Value: "", Path: "/", MaxAge: -1})
}

// --- Public: Login ---

func (s *Server) handleAPIPasskeysLoginBegin(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		http.Error(w, "passkeys not configured", http.StatusNotImplemented)
		return
	}
	options, sd, err := s.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		slog.Error("passkey login begin", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.storeWASession(w, r, "login", nil, sd); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (s *Server) handleAPIPasskeysLoginFinish(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		http.Error(w, "passkeys not configured", http.StatusNotImplemented)
		return
	}
	sd, err := s.fetchWASession(r, "login")
	if err != nil {
		clearWACookie(w)
		http.Error(w, "invalid or expired session", http.StatusBadRequest)
		return
	}
	clearWACookie(w)

	var foundUser *db.User
	handler := func(rawID, userHandle []byte) (wawebauthn.User, error) {
		creds, userID, err := s.db.GetCredentialsByWebAuthnID(r.Context(), userHandle)
		if err != nil {
			return nil, fmt.Errorf("user not found")
		}
		u, err := s.db.GetUserByID(r.Context(), userID)
		if err != nil {
			return nil, err
		}
		foundUser = u
		return &waUser{user: u, creds: creds}, nil
	}

	credential, err := s.webAuthn.FinishDiscoverableLogin(handler, *sd, r)
	if err != nil {
		slog.Warn("passkey login finish", "err", err)
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}
	if foundUser == nil {
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	credJSON, _ := json.Marshal(credential)
	_ = s.db.UpdateCredentialUsed(r.Context(), credential.ID, string(credJSON))

	if err := s.sessions.SetSession(w, foundUser.ID, foundUser.Username, foundUser.Permission, foundUser.FullName, foundUser.AvatarURL); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"redirect": "/"})
}

// --- Protected: Registration ---

func (s *Server) handleAPIPasskeysRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		http.Error(w, "passkeys not configured", http.StatusNotImplemented)
		return
	}
	sess := auth.FromContext(r.Context())
	wu, err := s.buildWAUser(r, sess.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	options, sd, err := s.webAuthn.BeginRegistration(wu,
		wawebauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	)
	if err != nil {
		slog.Error("passkey register begin", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := s.storeWASession(w, r, "registration", &sess.UserID, sd); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (s *Server) handleAPIPasskeysRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if s.webAuthn == nil {
		http.Error(w, "passkeys not configured", http.StatusNotImplemented)
		return
	}
	sd, err := s.fetchWASession(r, "registration")
	if err != nil {
		clearWACookie(w)
		http.Error(w, "invalid or expired session", http.StatusBadRequest)
		return
	}
	clearWACookie(w)

	sess := auth.FromContext(r.Context())
	wu, err := s.buildWAUser(r, sess.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	credential, err := s.webAuthn.FinishRegistration(wu, *sd, r)
	if err != nil {
		slog.Warn("passkey register finish", "err", err)
		http.Error(w, "registration failed", http.StatusBadRequest)
		return
	}
	credJSON, _ := json.Marshal(credential)

	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Passkey"
	}
	cred, err := s.db.CreateCredential(r.Context(), sess.UserID, credential.ID, name, string(credJSON))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":         cred.ID,
		"name":       cred.Name,
		"created_at": cred.CreatedAt,
	})
}

// --- Protected: List / Rename / Delete ---

func (s *Server) handleAPIListPasskeys(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	creds, err := s.db.ListCredentials(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	type row struct {
		ID         int64      `json:"id"`
		Name       string     `json:"name"`
		CreatedAt  time.Time  `json:"created_at"`
		LastUsedAt *time.Time `json:"last_used_at"`
	}
	out := make([]row, len(creds))
	for i, c := range creds {
		out[i] = row{c.ID, c.Name, c.CreatedAt, c.LastUsedAt}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleAPIRenamePasskey(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.db.RenameCredential(r.Context(), id, sess.UserID, body.Name); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPIDeletePasskey(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	// Guard: block deletion if this is the last passkey and user has no password.
	n, err := s.db.CountCredentials(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if n <= 1 {
		u, err := s.db.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if u.Password == "" || u.Password == "*" {
			http.Error(w, "cannot delete last passkey when no password is set", http.StatusConflict)
			return
		}
	}
	if err := s.db.DeleteCredential(r.Context(), id, sess.UserID); err != nil {
		if err == db.ErrNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
