package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newManager() *Manager {
	return NewManager(make([]byte, 32), false)
}

// sessionRequest builds a request carrying the session cookie set for the given user.
func sessionRequest(t *testing.T, m *Manager, id int64, username, permission string) *http.Request {
	t.Helper()
	w := httptest.NewRecorder()
	if err := m.SetSession(w, id, username, permission, "", ""); err != nil {
		t.Fatalf("SetSession: %v", err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestSessionRoundtrip(t *testing.T) {
	m := newManager()
	req := sessionRequest(t, m, 1, "alice", "admin")

	sess := m.GetSession(req)
	if sess == nil {
		t.Fatal("GetSession returned nil")
	}
	if sess.UserID != 1 || sess.Username != "alice" || sess.Permission != "admin" {
		t.Errorf("unexpected session: %+v", sess)
	}
}

func TestSessionTamperedSignature(t *testing.T) {
	m := newManager()
	w := httptest.NewRecorder()
	m.SetSession(w, 1, "alice", "superuser", "", "") //nolint:errcheck

	req := httptest.NewRequest("GET", "/", nil)
	for _, c := range w.Result().Cookies() {
		v := []byte(c.Value)
		v[len(v)-1] ^= 0xFF
		c.Value = string(v)
		req.AddCookie(c)
	}
	if sess := m.GetSession(req); sess != nil {
		t.Errorf("expected nil for tampered cookie, got %+v", sess)
	}
}

func TestSessionClear(t *testing.T) {
	m := newManager()
	// Use separate recorders so we only inspect the clear cookie.
	w := httptest.NewRecorder()
	m.ClearSession(w)

	found := false
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			found = true
			if c.MaxAge >= 0 {
				t.Errorf("expected MaxAge < 0 after clear, got %d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("no session cookie in clear response")
	}
}

func TestRequirePermission_noSession(t *testing.T) {
	m := newManager()
	handler := m.Middleware(m.RequirePermission("readonly")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

func TestRequirePermission_insufficientRole(t *testing.T) {
	m := newManager()
	handler := m.Middleware(m.RequirePermission("readwrite")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := sessionRequest(t, m, 2, "viewer", "readonly")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequirePermission_sufficientRole(t *testing.T) {
	m := newManager()
	var called bool
	handler := m.Middleware(m.RequirePermission("readwrite")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))

	req := sessionRequest(t, m, 3, "editor", "readwrite")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("handler was not called")
	}
}
