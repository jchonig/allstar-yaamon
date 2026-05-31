package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"allstar-yaamon/internal/config"
)

// loadTestConfig writes yaml to a temp file and loads it via config.Load so
// that defaults are applied correctly.
func loadTestConfig(t *testing.T, yaml string) *config.Config {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(f, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// mockQUICHeaderSetter satisfies quicHeaderSetter without a live UDP listener.
type mockQUICHeaderSetter struct{ port string }

func (m *mockQUICHeaderSetter) SetQUICHeaders(hdr http.Header) error {
	hdr.Set("Alt-Svc", `h3="`+m.port+`"; ma=2592000`)
	return nil
}

// --- altSvcMiddleware ---

func TestAltSvcMiddleware_SetsHeader(t *testing.T) {
	mock := &mockQUICHeaderSetter{port: ":443"}
	handler := altSvcMiddleware(mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	altSvc := w.Header().Get("Alt-Svc")
	if altSvc == "" {
		t.Fatal("Alt-Svc header not set")
	}
	if !strings.Contains(altSvc, "h3") {
		t.Errorf("Alt-Svc = %q, want it to contain 'h3'", altSvc)
	}
}

func TestAltSvcMiddleware_PassesThrough(t *testing.T) {
	mock := &mockQUICHeaderSetter{port: ":8443"}
	handler := altSvcMiddleware(mock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "yes")
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if w.Header().Get("X-Custom") != "yes" {
		t.Error("downstream response header not preserved")
	}
}

// --- QUIC config default ---

func TestServerConfig_QUICDefault(t *testing.T) {
	cfg := loadTestConfig(t, "")
	if !cfg.Server.QUIC {
		t.Error("server.quic should default to true")
	}
}

func TestServerConfig_QUICDisable(t *testing.T) {
	cfg := loadTestConfig(t, "server:\n  quic: false\n")
	if cfg.Server.QUIC {
		t.Error("server.quic should be false when explicitly disabled")
	}
}
