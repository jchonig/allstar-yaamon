package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"allstar-yaamon/internal/auth"
	"allstar-yaamon/internal/config"
	"allstar-yaamon/internal/db"
)

// --- highestRole ---

func TestHighestRole(t *testing.T) {
	roles := map[string]string{
		"admins":   "admin",
		"writers":  "readwrite",
		"readers":  "readonly",
		"supers":   "superuser",
	}

	tests := []struct {
		groups   string
		wantRole string
		wantOK   bool
	}{
		{"readers", "readonly", true},
		{"writers", "readwrite", true},
		{"admins", "admin", true},
		{"supers", "superuser", true},
		{"readers,admins", "admin", true},   // highest wins
		{"writers,supers", "superuser", true},
		{"unknown", "", false},
		{"", "", false},
		{"unknown,readers", "readonly", true},
	}

	for _, tc := range tests {
		role, ok := highestRole(tc.groups, roles)
		if ok != tc.wantOK || role != tc.wantRole {
			t.Errorf("highestRole(%q) = (%q, %v), want (%q, %v)",
				tc.groups, role, ok, tc.wantRole, tc.wantOK)
		}
	}
}

func TestHighestRole_EmptyMap(t *testing.T) {
	_, ok := highestRole("any-group", nil)
	if ok {
		t.Error("expected false for nil group map")
	}
}

// --- splitLogins ---

func TestSplitLogins(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a@ts", []string{"a@ts"}},
		{"a@ts,b@ts", []string{"a@ts", "b@ts"}},
		{"a@ts, b@ts", []string{"a@ts", "b@ts"}},   // spaces trimmed
		{"a@ts,a@ts", []string{"a@ts"}},             // deduplication
		{",a@ts,", []string{"a@ts"}},                // empty parts dropped
		{"a@ts,b@ts,a@ts", []string{"a@ts", "b@ts"}}, // dedup preserves first seen
	}
	for _, tc := range tests {
		got := splitLogins(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitLogins(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitLogins(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// --- proxyAuthMiddleware integration ---

func newProxyTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestProxyAuthMiddleware_OAuth2_Denied_NoGroup(t *testing.T) {
	database := newProxyTestDB(t)
	cfg := config.ProxyAuthConfig{
		Enabled:        true,
		UsernameHeader: "X-Auth-Request-Preferred-Username",
		GroupsHeader:   "X-Auth-Request-Groups",
		GroupRoles:     map[string]string{"admins": "admin"},
	}
	mw := proxyAuthMiddleware(cfg, config.TailscaleAuthConfig{}, database)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Auth-Request-Preferred-Username", "jch")
	req.Header.Set("X-Auth-Request-Groups", "other-group")

	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestProxyAuthMiddleware_OAuth2_SessionSet(t *testing.T) {
	database := newProxyTestDB(t)
	_, err := database.CreateUser(context.Background(), "jch", "*", db.PermReadOnly)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	cfg := config.ProxyAuthConfig{
		Enabled:        true,
		UsernameHeader: "X-Auth-Request-Preferred-Username",
		GroupsHeader:   "X-Auth-Request-Groups",
		GroupRoles:     map[string]string{"admins": "admin"},
		CreateUsers:    false,
	}
	mw := proxyAuthMiddleware(cfg, config.TailscaleAuthConfig{}, database)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Auth-Request-Preferred-Username", "jch")
	req.Header.Set("X-Auth-Request-Groups", "admins")

	var capturedSess *auth.Session
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSess = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if capturedSess == nil {
		t.Fatal("expected session in context, got nil")
	}
	if capturedSess.Username != "jch" {
		t.Errorf("expected username jch, got %s", capturedSess.Username)
	}
	if capturedSess.Permission != "admin" {
		t.Errorf("expected permission admin, got %s", capturedSess.Permission)
	}
	if capturedSess.AuthMethod != "OAuth2" {
		t.Errorf("expected AuthMethod OAuth2, got %s", capturedSess.AuthMethod)
	}
}

func TestProxyAuthMiddleware_Tailscale_SessionSet(t *testing.T) {
	database := newProxyTestDB(t)
	u, _ := database.CreateUser(context.Background(), "alice", "hash", db.PermReadWrite)
	_ = database.SetTailscaleLogins(context.Background(), u.ID, []string{"alice@github"})

	tsCfg := config.TailscaleAuthConfig{
		Enabled:    true,
		UserHeader: "Tailscale-User-Login",
	}
	mw := proxyAuthMiddleware(config.ProxyAuthConfig{}, tsCfg, database)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "alice@github")

	var capturedSess *auth.Session
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSess = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if capturedSess == nil {
		t.Fatal("expected session in context, got nil")
	}
	if capturedSess.Username != "alice" {
		t.Errorf("expected username alice, got %s", capturedSess.Username)
	}
	if capturedSess.AuthMethod != "tailscale" {
		t.Errorf("expected AuthMethod tailscale, got %s", capturedSess.AuthMethod)
	}
}

func TestProxyAuthMiddleware_Tailscale_NoMatch_FallsThrough(t *testing.T) {
	database := newProxyTestDB(t)
	tsCfg := config.TailscaleAuthConfig{
		Enabled:    true,
		UserHeader: "Tailscale-User-Login",
	}
	mw := proxyAuthMiddleware(config.ProxyAuthConfig{}, tsCfg, database)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Tailscale-User-Login", "nobody@github")

	var capturedSess *auth.Session
	rr := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSess = auth.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (fall through), got %d", rr.Code)
	}
	if capturedSess != nil {
		t.Errorf("expected nil session on no match, got %+v", capturedSess)
	}
}
