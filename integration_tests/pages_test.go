//go:build integration

package integration_tests

import (
	"net/http"
	"strings"
	"testing"
)

func TestAdminPagesLoad(t *testing.T) {
	c := adminClient(t)
	pages := []string{"/dashboard", "/admin/nodes", "/admin/users", "/admin/backup"}
	for _, path := range pages {
		t.Run(path, func(t *testing.T) {
			resp := do(t, c, http.MethodGet, path, nil)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
				t.Errorf("expected text/html Content-Type, got %q", ct)
			}
		})
	}
}

func TestViewerCanAccessDashboard(t *testing.T) {
	c := viewerClient(t)
	resp := do(t, c, http.MethodGet, "/dashboard", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("viewer /dashboard: expected 200, got %d", resp.StatusCode)
	}
}

func TestStaticAssetsAccessible(t *testing.T) {
	assets := []string{"/static/app.css"}
	for _, path := range assets {
		t.Run(path, func(t *testing.T) {
			resp := get(t, path)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		})
	}
}
