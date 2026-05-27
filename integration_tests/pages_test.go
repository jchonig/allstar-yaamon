//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAdminPagesLoad(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)
	// Use the node-specific dashboard URL; /dashboard redirects to /dashboard/{id}
	// for single-node installs so we go directly to avoid following redirects.
	pages := []string{fmt.Sprintf("/dashboard/%d", nid), "/admin/nodes", "/admin/users", "/admin/backup"}
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
	nid := seedNodeID(t)
	resp := do(t, c, http.MethodGet, fmt.Sprintf("/dashboard/%d", nid), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("viewer /dashboard/%d: expected 200, got %d", nid, resp.StatusCode)
	}
}

func TestDashboardOverviewSetsRedirectsToDashboard(t *testing.T) {
	c := adminClient(t)
	// /dashboard/overview should redirect back to /dashboard.
	resp := do(t, c, http.MethodGet, "/dashboard/overview", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		t.Errorf("/dashboard/overview: expected redirect, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/dashboard" {
		t.Errorf("/dashboard/overview: Location = %q, want /dashboard", loc)
	}
}

func TestDashboardOverviewSetsCookie(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodGet, "/dashboard/overview", nil)
	defer resp.Body.Close()

	var found bool
	for _, ck := range resp.Cookies() {
		if ck.Name == "yaamon_last_dashboard" {
			found = true
			if ck.Value != "overview" {
				t.Errorf("cookie value = %q, want overview", ck.Value)
			}
		}
	}
	if !found {
		t.Error("yaamon_last_dashboard cookie not set by /dashboard/overview")
	}
}

func TestDashboardNodePageSetsCookie(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)
	resp := do(t, c, http.MethodGet, fmt.Sprintf("/dashboard/%d", nid), nil)
	defer resp.Body.Close()

	var found bool
	for _, ck := range resp.Cookies() {
		if ck.Name == "yaamon_last_dashboard" {
			found = true
			want := fmt.Sprintf("%d", nid)
			if ck.Value != want {
				t.Errorf("cookie value = %q, want %q", ck.Value, want)
			}
		}
	}
	if !found {
		t.Error("yaamon_last_dashboard cookie not set by /dashboard/{nodeID}")
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
