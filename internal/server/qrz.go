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
	"allstar-yaamon/internal/qrz"
)

var validCallsign = regexp.MustCompile(`(?i)^[a-z0-9]{3,10}$`)

// handleAPIQRZLookup returns cached QRZ data for a callsign.
// GET /api/qrz/{callsign}
func (s *Server) handleAPIQRZLookup(w http.ResponseWriter, r *http.Request) {
	if s.qrzClient == nil || !s.qrzClient.Configured() {
		http.Error(w, "QRZ not configured", http.StatusServiceUnavailable)
		return
	}
	call := chi.URLParam(r, "callsign")
	if !validCallsign.MatchString(call) {
		http.Error(w, "invalid callsign", http.StatusBadRequest)
		return
	}
	rec, err := s.qrzClient.Lookup(r.Context(), call, s.db)
	if err != nil {
		http.Error(w, "QRZ lookup failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, rec)
}

// handleAPIGetQRZCredentials returns the stored QRZ username (password is never returned).
// GET /api/admin/integrations/qrz
func (s *Server) handleAPIGetQRZCredentials(w http.ResponseWriter, r *http.Request) {
	username, _ := s.db.GetConfig(r.Context(), "qrz_username")
	configured := username != ""
	writeJSON(w, map[string]any{
		"username":   username,
		"configured": configured,
	})
}

// handleAPISetQRZCredentials saves QRZ credentials and re-initialises the client.
// PUT /api/admin/integrations/qrz
// Body: {"username": "...", "password": "..."}
func (s *Server) handleAPISetQRZCredentials(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := s.db.SetConfig(ctx, "qrz_username", body.Username); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	encryptedPass, err := encryptQRZPassword(s.cipherKey, body.Password)
	if err != nil {
		http.Error(w, "encryption error", http.StatusInternalServerError)
		return
	}
	if err := s.db.SetConfig(ctx, "qrz_password_enc", encryptedPass); err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	s.qrzClient = qrz.New(body.Username, body.Password)
	s.seedQRZCache(ctx)

	writeJSON(w, map[string]any{"ok": true})
}

// handleAPIDeleteQRZCredentials removes QRZ credentials.
// DELETE /api/admin/integrations/qrz
func (s *Server) handleAPIDeleteQRZCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.db.SetConfig(ctx, "qrz_username", "")    //nolint:errcheck
	s.db.SetConfig(ctx, "qrz_password_enc", "") //nolint:errcheck
	s.qrzClient = nil
	writeJSON(w, map[string]any{"ok": true})
}

// handleIntegrationsPage renders the Integrations admin page.
func (s *Server) handleIntegrationsPage(w http.ResponseWriter, r *http.Request) {
	sess := auth.FromContext(r.Context())
	data := struct {
		pageData
		QRZConfigured bool
		QRZUsername   string
	}{pageData: s.newPageData()}
	fillSession(&data.pageData, sess)
	data.QRZUsername, _ = s.db.GetConfig(r.Context(), "qrz_username")
	data.QRZConfigured = data.QRZUsername != ""
	s.render(w, "integrations", data)
}

// seedQRZCache loads the DB cache into the in-memory client.
func (s *Server) seedQRZCache(ctx context.Context) {
	if s.qrzClient == nil {
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
	s.qrzClient.Seed(records)
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

// initQRZ loads QRZ credentials from the DB and seeds the in-memory cache.
func (s *Server) initQRZ(ctx context.Context) {
	username, _ := s.db.GetConfig(ctx, "qrz_username")
	encPass, _ := s.db.GetConfig(ctx, "qrz_password_enc")
	if username == "" || encPass == "" {
		return
	}
	password, err := decryptQRZPassword(s.cipherKey, encPass)
	if err != nil {
		return
	}
	s.qrzClient = qrz.New(username, password)
	s.seedQRZCache(ctx)
}

// deriveQRZKey derives a 32-byte AES key from the session secret.
func deriveQRZKey(sessionSecret []byte) [32]byte {
	return sha256.Sum256(append([]byte("qrz:"), sessionSecret...))
}
