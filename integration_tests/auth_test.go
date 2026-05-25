//go:build integration

package integration_tests

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHealth(t *testing.T) {
	resp := get(t, "/health")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]string
	decodeJSON(t, resp, &body)
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
}

func TestRootRedirect(t *testing.T) {
	resp := get(t, "/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/dashboard" {
		t.Fatalf("expected Location: /dashboard, got %q", loc)
	}
}

func TestLoginPageLoads(t *testing.T) {
	resp := get(t, "/login")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestLoginSuccess(t *testing.T) {
	c := adminClient(t) // panics via t.Fatalf if login fails
	// Verify the session works — /dashboard should return 200.
	resp := do(t, c, http.MethodGet, "/dashboard", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after login, got %d", resp.StatusCode)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {"admin"},
		"password": {"definitely-wrong"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLoginUnknownUser(t *testing.T) {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {"nobody"},
		"password": {"password"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLogout(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodGet, "/logout", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
	// Confirm session is cleared — /dashboard should now redirect.
	resp2 := do(t, c, http.MethodGet, "/dashboard", nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusFound {
		t.Errorf("after logout, /dashboard should redirect, got %d", resp2.StatusCode)
	}
}

func TestUnauthenticatedRedirectsToLogin(t *testing.T) {
	protected := []string{"/dashboard", "/admin/nodes", "/admin/users", "/admin/backup"}
	for _, path := range protected {
		t.Run(path, func(t *testing.T) {
			resp := get(t, path)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusFound {
				t.Errorf("%s: expected 302, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestViewerBlockedFromAdmin(t *testing.T) {
	c := viewerClient(t)
	adminPaths := []string{"/admin/nodes", "/admin/users", "/admin/backup"}
	for _, path := range adminPaths {
		t.Run(path, func(t *testing.T) {
			resp := do(t, c, http.MethodGet, path, nil)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s: expected 403, got %d", path, resp.StatusCode)
			}
		})
	}
}

func TestViewerBlockedFromAdminAPIs(t *testing.T) {
	c := viewerClient(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPost, "/api/nodes", map[string]any{"name": "x", "node_number": "1"}},
		{http.MethodPost, "/api/users", map[string]any{"username": "x", "password": "y", "permission": "readonly"}},
		{http.MethodPost, "/api/backup", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := do(t, c, tc.method, tc.path, tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("expected 403, got %d", resp.StatusCode)
			}
		})
	}
}
