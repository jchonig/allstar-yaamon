package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/db"
)

func encodeB64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
func itoa(id int64) string      { return strconv.FormatInt(id, 10) }

func withChiParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// newProfileTestServer creates a minimal Server wired to a fresh SQLite DB.
func newProfileTestServer(t *testing.T) (*Server, *db.DB, *auth.Manager) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	mgr := auth.NewManager(make([]byte, 32), false)
	s := &Server{
		db:         database,
		sessions:   mgr,
		statsCache: newStatsCache(),
		linksCache: newLinksCache(),
	}
	return s, database, mgr
}

// seedUser creates a user and returns it with a bcrypt hash.
func seedUser(t *testing.T, database *db.DB, username, password, permission string) *db.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u, err := database.CreateUser(t.Context(), username, hash, permission)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u.Password = hash
	return u
}

// authedReq builds an HTTP request with a valid session cookie injected into context.
func authedReq(t *testing.T, mgr *auth.Manager, id int64, username, permission, fullName, avatarURL, method, target string, body io.Reader) *http.Request {
	t.Helper()
	w := httptest.NewRecorder()
	if err := mgr.SetSession(w, id, username, permission, fullName, avatarURL); err != nil {
		t.Fatalf("SetSession: %v", err)
	}
	req := httptest.NewRequest(method, target, body)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	// Run through Middleware so the session lands in context.
	var out *http.Request
	mgr.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		out = r
	})).ServeHTTP(httptest.NewRecorder(), req)
	return out
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return bytes.NewReader(b)
}

// --- validateSessionUser ---

func TestValidateSessionUser_ValidUser(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	var called bool
	handler := s.validateSessionUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called for a valid user")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestValidateSessionUser_DeletedUser(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "ghost", "password123", db.PermReadOnly)

	// Delete the user before the request arrives.
	if err := database.DeleteUser(t.Context(), u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	var called bool
	handler := s.validateSessionUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called for a deleted user")
	}
	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect to /login", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestValidateSessionUser_DeletedUser_API(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "ghost2", "password123", db.PermReadOnly)
	database.DeleteUser(t.Context(), u.ID) //nolint:errcheck

	handler := s.validateSessionUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET", "/api/profile", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("API path: status = %d, want 401", w.Code)
	}
}

// --- handleAPIUpdateProfile ---

func TestHandleAPIUpdateProfile_FullName(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile",
		jsonBody(map[string]string{"full_name": "Alice Smith"}))
	w := httptest.NewRecorder()
	s.handleAPIUpdateProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if got.FullName != "Alice Smith" {
		t.Errorf("FullName = %q, want %q", got.FullName, "Alice Smith")
	}
}

func TestHandleAPIUpdateProfile_PasswordChange_Success(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "oldpassword1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile",
		jsonBody(map[string]string{
			"current_password": "oldpassword1",
			"new_password":     "newpassword123",
		}))
	w := httptest.NewRecorder()
	s.handleAPIUpdateProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if err := auth.CheckPassword(got.Password, "newpassword123"); err != nil {
		t.Errorf("new password does not match: %v", err)
	}
}

func TestHandleAPIUpdateProfile_PasswordChange_WrongCurrent(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "realpassword1", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile",
		jsonBody(map[string]string{
			"current_password": "wrongpassword1",
			"new_password":     "newpassword123",
		}))
	w := httptest.NewRecorder()
	s.handleAPIUpdateProfile(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandleAPIUpdateProfile_ExternalAvatarURL_ClearsUploadedAvatar(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	// Pre-store an uploaded avatar.
	key := avatarConfigKey(u.ID)
	database.SetConfig(t.Context(), key, "ZmFrZXBuZw==") //nolint:errcheck

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "PUT", "/api/profile",
		jsonBody(map[string]string{"avatar_url": "https://example.com/avatar.jpg"}))
	w := httptest.NewRecorder()
	s.handleAPIUpdateProfile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	// Uploaded avatar data should be cleared.
	val, _ := database.GetConfig(t.Context(), key)
	if val != "" {
		t.Errorf("uploaded avatar data should be cleared, got %q", val)
	}
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if got.AvatarURL != "https://example.com/avatar.jpg" {
		t.Errorf("AvatarURL = %q, want external URL", got.AvatarURL)
	}
}

// --- handleAPIUploadAvatar ---

func pngBytes() []byte {
	// Minimal bytes that pass the PNG magic check (first 4 bytes are PNG signature).
	return []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
}

func TestHandleAPIUploadAvatar_PNG(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "POST", "/api/profile/avatar",
		bytes.NewReader(pngBytes()))
	req.Header.Set("Content-Type", "image/png")
	w := httptest.NewRecorder()
	s.handleAPIUploadAvatar(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	// Config should have the base64 encoded bytes.
	val, _ := database.GetConfig(t.Context(), avatarConfigKey(u.ID))
	if val == "" {
		t.Error("avatar data should be stored in configs")
	}
	// DB avatar_url should point to the API endpoint.
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if !strings.HasPrefix(got.AvatarURL, "/api/users/") {
		t.Errorf("AvatarURL = %q, want /api/users/... path", got.AvatarURL)
	}
}

func TestHandleAPIUploadAvatar_InvalidFormat(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "POST", "/api/profile/avatar",
		bytes.NewReader([]byte("not an image at all")))
	req.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	s.handleAPIUploadAvatar(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid format", w.Code)
	}
	val, _ := database.GetConfig(t.Context(), avatarConfigKey(u.ID))
	if val != "" {
		t.Error("invalid file should not be stored")
	}
}

// --- handleAPIDeleteAvatar ---

func TestHandleAPIDeleteAvatar(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	// Seed an avatar.
	database.SetConfig(t.Context(), avatarConfigKey(u.ID), "ZmFrZXBuZw==") //nolint:errcheck
	database.UpdateUserProfile(t.Context(), u.ID, "", "/api/users/1/avatar")  //nolint:errcheck

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "/api/users/1/avatar",
		"DELETE", "/api/profile/avatar", nil)
	w := httptest.NewRecorder()
	s.handleAPIDeleteAvatar(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	val, _ := database.GetConfig(t.Context(), avatarConfigKey(u.ID))
	if val != "" {
		t.Error("avatar data should be cleared after delete")
	}
	got, _ := database.GetUserByID(t.Context(), u.ID)
	if got.AvatarURL != "" {
		t.Errorf("AvatarURL = %q, want empty after delete", got.AvatarURL)
	}
}

// --- handleAPIGetAvatar ---

func TestHandleAPIGetAvatar_Found(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	encoded := encodeB64(pngBytes())
	database.SetConfig(t.Context(), avatarConfigKey(u.ID), encoded) //nolint:errcheck

	// Build request with chi URL param.
	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET",
		"/api/users/"+itoa(u.ID)+"/avatar", nil)
	req = withChiParam(req, "id", itoa(u.ID))

	w := httptest.NewRecorder()
	s.handleAPIGetAvatar(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

func TestHandleAPIGetAvatar_NotFound(t *testing.T) {
	s, database, mgr := newProfileTestServer(t)
	u := seedUser(t, database, "alice", "password123", db.PermReadOnly)

	req := authedReq(t, mgr, u.ID, u.Username, u.Permission, "", "", "GET",
		"/api/users/"+itoa(u.ID)+"/avatar", nil)
	req = withChiParam(req, "id", itoa(u.ID))

	w := httptest.NewRecorder()
	s.handleAPIGetAvatar(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when no avatar stored", w.Code)
	}
}
