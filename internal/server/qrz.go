package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
	"allstar-yaamon/internal/qrz"
)

var validCallsign = regexp.MustCompile(`(?i)^[a-z0-9]{3,10}$`)

// userQRZClient returns the QRZ client for the given user, or nil if not configured.
func (s *Server) userQRZClient(user *db.User) *qrz.Client {
	if user.QRZUsername == "" || user.QRZPasswordEnc == "" {
		return nil
	}
	s.qrzMu.RLock()
	c, ok := s.qrzClients[user.ID]
	s.qrzMu.RUnlock()
	if ok {
		return c
	}
	// Client missing from map — recreate from stored credentials.
	password, err := decryptQRZPassword(s.cipherKey, user.QRZPasswordEnc)
	if err != nil {
		return nil
	}
	c = qrz.New(user.QRZUsername, password)
	s.qrzMu.Lock()
	s.qrzClients[user.ID] = c
	s.qrzMu.Unlock()
	return c
}

// handleAPIQRZLookup returns callsign data for a callsign using the requesting
// user's lookup source preference and QRZ credentials (if any).
// GET /api/qrz/{callsign}
func (s *Server) handleAPIQRZLookup(w http.ResponseWriter, r *http.Request) {
	call := chi.URLParam(r, "callsign")
	if !validCallsign.MatchString(call) {
		http.Error(w, "invalid callsign", http.StatusBadRequest)
		return
	}

	sess := auth.FromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	source := user.LookupSource
	if source == "" {
		source = "auto"
	}

	var (
		rec     qrz.Record
		lookErr error
	)
	switch source {
	case "qrz":
		c := s.userQRZClient(user)
		if c == nil {
			http.Error(w, "QRZ not configured", http.StatusServiceUnavailable)
			return
		}
		rec, lookErr = c.Lookup(r.Context(), call, nil) // nil: QRZ data is per-user, not cached to DB
	case "callook":
		rec, lookErr = s.callookClient.LookupCallook(r.Context(), call, s.db)
	default: // "auto": QRZ when configured, callook otherwise
		if c := s.userQRZClient(user); c != nil {
			rec, lookErr = c.Lookup(r.Context(), call, nil)
		} else {
			rec, lookErr = s.callookClient.LookupCallook(r.Context(), call, s.db)
		}
	}

	if lookErr != nil {
		http.Error(w, "lookup failed: "+lookErr.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, rec)
}

// handleAPIClearQRZCache deletes all cached callsign records from the DB and in-memory caches.
// DELETE /api/admin/integrations/qrz/cache
func (s *Server) handleAPIClearQRZCache(w http.ResponseWriter, r *http.Request) {
	if err := s.db.ClearQRZCache(r.Context()); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	s.callookClient.ClearCache()
	s.qrzMu.RLock()
	for _, c := range s.qrzClients {
		c.ClearCache()
	}
	s.qrzMu.RUnlock()
	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIGetUserQRZ returns the current user's QRZ username and configured status.
// GET /api/profile/qrz
func (s *Server) handleAPIGetUserQRZ(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]any{
		"username":   user.QRZUsername,
		"configured": user.QRZUsername != "" && user.QRZPasswordEnc != "",
	})
}

// handleAPISetUserQRZ saves QRZ credentials for the current user.
// PUT /api/profile/qrz
// Body: {"username": "...", "password": "..."}  — password may be omitted to keep current.
func (s *Server) handleAPISetUserQRZ(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}

	sess := auth.FromContext(r.Context())
	user, err := s.db.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Use existing encrypted password if none supplied.
	encPass := user.QRZPasswordEnc
	if body.Password != "" {
		enc, err := encryptQRZPassword(s.cipherKey, body.Password)
		if err != nil {
			http.Error(w, "encryption error", http.StatusInternalServerError)
			return
		}
		encPass = enc
	}
	if encPass == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateUserQRZ(r.Context(), user.ID, body.Username, encPass); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Decrypt the password to configure the in-memory client.
	password, err := decryptQRZPassword(s.cipherKey, encPass)
	if err != nil {
		http.Error(w, "credential error", http.StatusInternalServerError)
		return
	}
	c := qrz.New(body.Username, password)
	s.qrzMu.Lock()
	s.qrzClients[user.ID] = c
	s.qrzMu.Unlock()

	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIDeleteUserQRZ removes QRZ credentials for the current user.
// DELETE /api/profile/qrz
func (s *Server) handleAPIDeleteUserQRZ(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	if err := s.db.UpdateUserQRZ(r.Context(), sess.UserID, "", ""); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	s.qrzMu.Lock()
	delete(s.qrzClients, sess.UserID)
	s.qrzMu.Unlock()
	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIClearUserQRZCache clears the current user's in-memory QRZ cache.
// DELETE /api/profile/qrz/cache
func (s *Server) handleAPIClearUserQRZCache(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	s.qrzMu.RLock()
	c, ok := s.qrzClients[sess.UserID]
	s.qrzMu.RUnlock()
	if ok {
		c.ClearCache()
	}
	writeJSON(w, map[string]any{"ok": true})
}

// seedQRZCache loads the DB cache into the shared callook client's in-memory cache.
func (s *Server) seedQRZCache(ctx context.Context) {
	if s.callookClient == nil {
		return
	}
	rows, err := s.db.LoadQRZCache(ctx)
	if err != nil {
		return
	}
	records := make(map[string]qrz.Record, len(rows))
	for call, raw := range rows {
		var rec qrz.Record
		if json.Unmarshal(raw, &rec) == nil {
			records[call] = rec
		}
	}
	s.callookClient.Seed(records)
}

// encryptQRZPassword encrypts plaintext using AES-256-GCM.
// Returns base64(nonce + ciphertext).
func encryptQRZPassword(key [32]byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// decryptQRZPassword decrypts a value produced by encryptQRZPassword.
func decryptQRZPassword(key [32]byte, encoded string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ct) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := ct[:gcm.NonceSize()], ct[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// initQRZ loads per-user QRZ credentials from the DB and seeds the callook cache.
// The callook client is always available even without any user credentials.
func (s *Server) initQRZ(ctx context.Context) {
	users, err := s.db.ListUsers(ctx)
	if err == nil {
		for _, u := range users {
			if u.QRZUsername == "" || u.QRZPasswordEnc == "" {
				continue
			}
			password, err := decryptQRZPassword(s.cipherKey, u.QRZPasswordEnc)
			if err != nil {
				continue
			}
			s.qrzClients[u.ID] = qrz.New(u.QRZUsername, password)
		}
	}
	s.seedQRZCache(ctx)
}

// deriveQRZKey derives a 32-byte AES key from the session secret.
func deriveQRZKey(sessionSecret []byte) [32]byte {
	return sha256.Sum256(append([]byte("qrz:"), sessionSecret...))
}
