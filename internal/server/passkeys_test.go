package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	wawebauthn "github.com/go-webauthn/webauthn/webauthn"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
	"allstar-yaamon/internal/qrz"
)

func newPasskeyTestServer(t *testing.T) (*Server, *db.DB, *auth.Manager) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	mgr := auth.NewManager(make([]byte, 32), false)
	wa, err := wawebauthn.New(&wawebauthn.Config{
		RPDisplayName: "Test",
		RPID:          "localhost",
		RPOrigins:     []string{"http://localhost"},
	})
	if err != nil {
		t.Fatalf("webauthn.New: %v", err)
	}
	s := &Server{
		cfg:           &config.Config{},
		db:            database,
		sessions:      mgr,
		statsCache:    newStatsCache(),
		linksCache:    newLinksCache(),
		callookClient: qrz.New("", ""),
		qrzClients:    make(map[int64]*qrz.Client),
		cipherKey:     deriveQRZKey(make([]byte, 32)),
		webAuthn:      wa,
	}
	return s, database, mgr
}

// --- handleAPIListPasskeys ---

func TestHandleAPIListPasskeys_Empty(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/api/passkeys", nil)
	w := httptest.NewRecorder()
	s.handleAPIListPasskeys(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var out []any
	json.NewDecoder(w.Body).Decode(&out) //nolint:errcheck
	if len(out) != 0 {
		t.Errorf("want empty list, got %d items", len(out))
	}
}

func TestHandleAPIListPasskeys_WithCredentials(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{}`) //nolint:errcheck
	database.CreateCredential(t.Context(), u.ID, []byte("cid2"), "YubiKey", `{}`)  //nolint:errcheck

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/api/passkeys", nil)
	w := httptest.NewRecorder()
	s.handleAPIListPasskeys(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var out []map[string]any
	json.NewDecoder(w.Body).Decode(&out) //nolint:errcheck
	if len(out) != 2 {
		t.Fatalf("want 2 passkeys, got %d", len(out))
	}
	if out[0]["name"] != "Touch ID" || out[1]["name"] != "YubiKey" {
		t.Errorf("unexpected names: %v, %v", out[0]["name"], out[1]["name"])
	}
}

// --- handleAPIRenamePasskey ---

func TestHandleAPIRenamePasskey(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	c, _ := database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{}`)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PATCH",
		"/api/passkeys/1", jsonBody(map[string]string{"name": "MacBook Pro"}))
	req = withChiParam(req, "id", itoa(c.ID))
	w := httptest.NewRecorder()
	s.handleAPIRenamePasskey(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	got, _ := database.GetCredential(t.Context(), c.ID, u.ID)
	if got.Name != "MacBook Pro" {
		t.Errorf("name = %q, want MacBook Pro", got.Name)
	}
}

func TestHandleAPIRenamePasskey_EmptyName(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	c, _ := database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{}`)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PATCH",
		"/api/passkeys/1", jsonBody(map[string]string{"name": ""}))
	req = withChiParam(req, "id", itoa(c.ID))
	w := httptest.NewRecorder()
	s.handleAPIRenamePasskey(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// --- handleAPIDeletePasskey ---

func TestHandleAPIDeletePasskey_WithPassword(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	c, _ := database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{}`)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/passkeys/1", nil)
	req = withChiParam(req, "id", itoa(c.ID))
	w := httptest.NewRecorder()
	s.handleAPIDeletePasskey(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	n, _ := database.CountCredentials(t.Context(), u.ID)
	if n != 0 {
		t.Errorf("want 0 credentials, got %d", n)
	}
}

func TestHandleAPIDeletePasskey_LastPasskeyNoPassword(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	// Create user with no real password (passkey-only sentinel).
	u, _ := database.CreateUser(t.Context(), "alice", "*", db.PermReadOnly)
	c, _ := database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{}`)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/passkeys/1", nil)
	req = withChiParam(req, "id", itoa(c.ID))
	w := httptest.NewRecorder()
	s.handleAPIDeletePasskey(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAPIDeletePasskey_LastPasskeyWithPassword(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	c, _ := database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{}`)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/passkeys/1", nil)
	req = withChiParam(req, "id", itoa(c.ID))
	w := httptest.NewRecorder()
	s.handleAPIDeletePasskey(w, req)

	// Last passkey can be deleted if user has a real password.
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAPIDeletePasskey_NotFound(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "DELETE", "/api/passkeys/999", nil)
	req = withChiParam(req, "id", "999")
	w := httptest.NewRecorder()
	s.handleAPIDeletePasskey(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// --- Passkey not configured ---

func TestPasskeyHandlers_NotConfigured(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	s.webAuthn = nil // disable
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)

	for _, tc := range []struct {
		method  string
		target  string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"POST", "/api/passkeys/login/begin", s.handleAPIPasskeysLoginBegin},
		{"POST", "/api/passkeys/login/finish", s.handleAPIPasskeysLoginFinish},
		{"POST", "/api/passkeys/register/begin", s.handleAPIPasskeysRegisterBegin},
		{"POST", "/api/passkeys/register/finish", s.handleAPIPasskeysRegisterFinish},
	} {
		t.Run(tc.target, func(t *testing.T) {
			req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", tc.method, tc.target, nil)
			w := httptest.NewRecorder()
			tc.handler(w, req)
			if w.Code != http.StatusNotImplemented {
				t.Errorf("status = %d, want 501", w.Code)
			}
		})
	}
}

// --- buildWAUser ---

func TestBuildWAUser(t *testing.T) {
	s, database, mgr := newPasskeyTestServer(t)
	u := seedUser(t, database, "alice", "password1", db.PermReadOnly)
	database.UpdateUserProfile(t.Context(), u.ID, "Alice Smith", "") //nolint:errcheck
	database.CreateCredential(t.Context(), u.ID, []byte("cid1"), "Touch ID", `{"id":"Y2lkMQ=="}`) //nolint:errcheck

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/", nil)
	wu, err := s.buildWAUser(req, u.ID)
	if err != nil {
		t.Fatalf("buildWAUser: %v", err)
	}
	if wu.WebAuthnName() != "alice" {
		t.Errorf("WebAuthnName = %q, want alice", wu.WebAuthnName())
	}
	if wu.WebAuthnDisplayName() != "Alice Smith" {
		t.Errorf("WebAuthnDisplayName = %q, want Alice Smith", wu.WebAuthnDisplayName())
	}
	if len(wu.WebAuthnID()) != 64 {
		t.Errorf("WebAuthnID length = %d, want 64", len(wu.WebAuthnID()))
	}
}

