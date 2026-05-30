package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
	"allstar-yaamon/internal/qrz"
)

// newQRZTestServer builds a server wired to a fresh DB with cipher key initialised.
func newQRZTestServer(t *testing.T) (*Server, *db.DB, *auth.Manager) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	mgr := auth.NewManager(make([]byte, 32), false)
	s := &Server{
		cfg:           &config.Config{},
		db:            database,
		sessions:      mgr,
		statsCache:    newStatsCache(),
		linksCache:    newLinksCache(),
		callookClient: qrz.New("", ""),
		qrzClients:    make(map[int64]*qrz.Client),
		cipherKey:     deriveQRZKey(make([]byte, 32)),
	}
	return s, database, mgr
}


// --- handleAPIGetUserQRZ ---

func TestHandleAPIGetUserQRZ_NotConfigured(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/api/profile/qrz", nil)
	w := httptest.NewRecorder()
	s.handleAPIGetUserQRZ(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["configured"] != false {
		t.Errorf("configured = %v, want false", resp["configured"])
	}
	if resp["username"] != "" {
		t.Errorf("username = %v, want empty", resp["username"])
	}
}

func TestHandleAPIGetUserQRZ_Configured(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	enc, _ := encryptQRZPassword(s.cipherKey, "secret")
	database.UpdateUserQRZ(t.Context(), u.ID, "W1AW", enc) //nolint:errcheck

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/api/profile/qrz", nil)
	w := httptest.NewRecorder()
	s.handleAPIGetUserQRZ(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["configured"] != true {
		t.Errorf("configured = %v, want true", resp["configured"])
	}
	if resp["username"] != "W1AW" {
		t.Errorf("username = %v, want W1AW", resp["username"])
	}
}

// --- handleAPISetUserQRZ ---

func TestHandleAPISetUserQRZ_NewCredentials(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile/qrz",
		jsonBody(map[string]string{"username": "W1AW", "password": "mysecret"}))
	w := httptest.NewRecorder()
	s.handleAPISetUserQRZ(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}

	got, _ := database.GetUserByID(t.Context(), u.ID)
	if got.QRZUsername != "W1AW" {
		t.Errorf("QRZUsername = %q, want W1AW", got.QRZUsername)
	}
	plain, err := decryptQRZPassword(s.cipherKey, got.QRZPasswordEnc)
	if err != nil || plain != "mysecret" {
		t.Errorf("stored password mismatch: plain=%q err=%v", plain, err)
	}

	// in-memory client should be populated
	s.qrzMu.RLock()
	_, ok := s.qrzClients[u.ID]
	s.qrzMu.RUnlock()
	if !ok {
		t.Error("qrzClients entry not created")
	}
}

func TestHandleAPISetUserQRZ_KeepExistingPassword(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	// Pre-set credentials.
	enc, _ := encryptQRZPassword(s.cipherKey, "original")
	database.UpdateUserQRZ(t.Context(), u.ID, "W1AW", enc) //nolint:errcheck

	// Update username only — no password in body.
	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile/qrz",
		jsonBody(map[string]string{"username": "W2AW"}))
	w := httptest.NewRecorder()
	s.handleAPISetUserQRZ(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if got.QRZUsername != "W2AW" {
		t.Errorf("QRZUsername = %q, want W2AW", got.QRZUsername)
	}
	plain, _ := decryptQRZPassword(s.cipherKey, got.QRZPasswordEnc)
	if plain != "original" {
		t.Errorf("password changed unexpectedly: %q", plain)
	}
}

func TestHandleAPISetUserQRZ_MissingUsername(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile/qrz",
		jsonBody(map[string]string{"password": "pw"}))
	w := httptest.NewRecorder()
	s.handleAPISetUserQRZ(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- handleAPIDeleteUserQRZ ---

func TestHandleAPIDeleteUserQRZ(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	enc, _ := encryptQRZPassword(s.cipherKey, "secret")
	database.UpdateUserQRZ(t.Context(), u.ID, "W1AW", enc) //nolint:errcheck
	s.qrzMu.Lock()
	s.qrzClients[u.ID] = qrz.New("W1AW", "secret")
	s.qrzMu.Unlock()

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/profile/qrz", nil)
	w := httptest.NewRecorder()
	s.handleAPIDeleteUserQRZ(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if got.QRZUsername != "" || got.QRZPasswordEnc != "" {
		t.Errorf("credentials not cleared in DB: %+v", got)
	}
	s.qrzMu.RLock()
	_, ok := s.qrzClients[u.ID]
	s.qrzMu.RUnlock()
	if ok {
		t.Error("qrzClients entry not removed")
	}
}

// --- handleAPIClearUserQRZCache ---

func TestHandleAPIClearUserQRZCache_NoClient(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/profile/qrz/cache", nil)
	w := httptest.NewRecorder()
	s.handleAPIClearUserQRZCache(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleAPIClearUserQRZCache_WithClient(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	c := qrz.New("W1AW", "secret")
	s.qrzMu.Lock()
	s.qrzClients[u.ID] = c
	s.qrzMu.Unlock()

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/profile/qrz/cache", nil)
	w := httptest.NewRecorder()
	s.handleAPIClearUserQRZCache(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- handleAPIUpdateProfile lookup_source ---

func TestHandleAPIUpdateProfile_LookupSource(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	for _, src := range []string{"auto", "qrz", "callook"} {
		req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile",
			jsonBody(map[string]string{"lookup_source": src}))
		w := httptest.NewRecorder()
		s.handleAPIUpdateProfile(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("source=%q: status = %d, want 200; body: %s", src, w.Code, w.Body.String())
		}
		got, _ := database.GetUserByID(t.Context(), u.ID)
		if got.LookupSource != src {
			t.Errorf("source=%q: LookupSource = %q", src, got.LookupSource)
		}
	}
}

func TestHandleAPIUpdateProfile_LookupSource_Invalid(t *testing.T) {
	s, database, mgr := newQRZTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile",
		jsonBody(map[string]string{"lookup_source": "invalid"}))
	w := httptest.NewRecorder()
	s.handleAPIUpdateProfile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- encryption round-trip ---

func TestEncryptDecryptQRZPassword(t *testing.T) {
	key := deriveQRZKey([]byte("test-session-secret"))
	plaintext := "super-secret-password!"

	enc, err := encryptQRZPassword(key, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plaintext {
		t.Fatal("ciphertext equals plaintext")
	}

	got, err := decryptQRZPassword(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("round-trip = %q, want %q", got, plaintext)
	}
}

func TestDecryptQRZPassword_WrongKey(t *testing.T) {
	key1 := deriveQRZKey([]byte("key-one"))
	key2 := deriveQRZKey([]byte("key-two"))

	enc, _ := encryptQRZPassword(key1, "secret")
	if _, err := decryptQRZPassword(key2, enc); err == nil {
		t.Error("expected error decrypting with wrong key")
	}
}
