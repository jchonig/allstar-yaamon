//go:build integration

package integration_tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// basepathURL is set from YAAMON_TEST_BASEPATH_URL (e.g. "http://yaamon-bp:80").
// basePath is set from YAAMON_TEST_BASEPATH (e.g. "/yaamon").
// If either is empty, all tests in this file are skipped.
var (
	basepathURL      string
	basePath         string
	basepathFreshURL string
)

func init() {
	basepathURL = os.Getenv("YAAMON_TEST_BASEPATH_URL")
	basePath = os.Getenv("YAAMON_TEST_BASEPATH")
	basepathFreshURL = os.Getenv("YAAMON_TEST_BASEPATH_FRESH_URL")
}

// bpURL returns the full URL for a path under the base path.
func bpURL(path string) string {
	return basepathURL + basePath + path
}

// bpFreshURL returns the full URL for a path under the base path on the fresh (no-users) server.
func bpFreshURL(path string) string {
	return basepathFreshURL + basePath + path
}

// skipIfNoBP skips the test if the base-path server is not configured.
func skipIfNoBP(t *testing.T) {
	t.Helper()
	if basepathURL == "" || basePath == "" {
		t.Skip("YAAMON_TEST_BASEPATH_URL or YAAMON_TEST_BASEPATH not set; skipping base-path tests")
	}
}

// skipIfNoBPFresh skips the test if the fresh base-path server is not configured.
func skipIfNoBPFresh(t *testing.T) {
	t.Helper()
	if basepathFreshURL == "" || basePath == "" {
		t.Skip("YAAMON_TEST_BASEPATH_FRESH_URL or YAAMON_TEST_BASEPATH not set; skipping fresh base-path tests")
	}
}

// waitForBPServer polls the /health endpoint (always at root, never under base_path).
func waitForBPServer(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(basepathURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("base-path server at %s not ready after 30s", basepathURL)
}

// waitForBPFreshServer polls /health on the fresh server.
func waitForBPFreshServer(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(basepathFreshURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("fresh base-path server at %s not ready after 30s", basepathFreshURL)
}

// loginBP logs in via the base-path prefixed login endpoint.
func loginBP(t *testing.T, username, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.PostForm(bpURL("/login"), url.Values{
		"username": {username},
		"password": {password},
	})
	if err != nil {
		t.Fatalf("loginBP POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("loginBP %q: expected 303, got %d", username, resp.StatusCode)
	}
	return client
}

// TestBasePath_Health verifies /health is served at the root (not under base_path).
func TestBasePath_Health(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	resp, err := http.Get(basepathURL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health: expected 200, got %d", resp.StatusCode)
	}
}

// TestBasePath_HealthNotUnderBasePath verifies /health is NOT served under the base path.
func TestBasePath_HealthNotUnderBasePath(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(bpURL("/health"))
	if err != nil {
		t.Fatalf("GET %s/health: %v", basePath, err)
	}
	resp.Body.Close()
	// Expect non-200 — health is not mounted under base_path.
	if resp.StatusCode == http.StatusOK {
		t.Errorf("%s/health: expected non-200 (should not be served under base_path), got 200", basePath)
	}
}

// TestBasePath_LoginPageServed verifies the login page loads under the base path.
func TestBasePath_LoginPageServed(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	resp, err := http.Get(bpURL("/login"))
	if err != nil {
		t.Fatalf("GET %s/login: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("%s/login: expected 200, got %d", basePath, resp.StatusCode)
	}
}

// TestBasePath_StaticAssets verifies static assets are served under the base path
// with the correct content types. This catches the http.StripPrefix bug where
// r.URL.Path includes the base_path prefix but the strip prefix did not.
func TestBasePath_StaticAssets(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	cases := []struct {
		path            string
		wantContentType string
	}{
		{"/static/app.css", "text/css"},
		{"/static/app.js", "application/javascript"},
		{"/favicon.png", "image/png"},
		{"/favicon.ico", "image/x-icon"},
	}
	for _, tc := range cases {
		resp, err := http.Get(bpURL(tc.path))
		if err != nil {
			t.Fatalf("GET %s%s: %v", basePath, tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s%s: expected 200, got %d", basePath, tc.path, resp.StatusCode)
			continue
		}
		if len(body) == 0 {
			t.Errorf("%s%s: response body is empty", basePath, tc.path)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, tc.wantContentType) {
			t.Errorf("%s%s: expected Content-Type containing %q, got %q", basePath, tc.path, tc.wantContentType, ct)
		}
	}
}

// TestBasePath_StaticAssetsNotAtRoot verifies static assets are not leaked at root paths.
func TestBasePath_StaticAssetsNotAtRoot(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	for _, path := range []string{"/static/app.css", "/static/app.js", "/favicon.png"} {
		resp, err := client.Get(basepathURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s: expected non-200 (should only be served under %s), got 200", path, basePath)
		}
	}
}

// TestBasePath_LoginRedirectsToDashboard verifies that a successful login
// redirects to the dashboard under the base path.
func TestBasePath_LoginRedirectsToDashboard(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.PostForm(bpURL("/login"), url.Values{
		"username": {"admin"},
		"password": {adminPassword},
	})
	if err != nil {
		t.Fatalf("POST %s/login: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, basePath+"/dashboard") {
		t.Errorf("login redirect: expected Location to start with %s/dashboard, got %q", basePath, loc)
	}
}

// TestBasePath_LogoutRedirectsToLogin verifies that logout redirects to login under the base path.
func TestBasePath_LogoutRedirectsToLogin(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := loginBP(t, "admin", adminPassword)
	resp, err := client.Get(bpURL("/logout"))
	if err != nil {
		t.Fatalf("GET %s/logout: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout: expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, basePath+"/login") {
		t.Errorf("logout redirect: expected Location to start with %s/login, got %q", basePath, loc)
	}
}

// TestBasePath_APIRequiresAuth verifies that API endpoints return 401 (not a redirect
// to a bare /login) for unauthenticated requests. This catches the profile.go bug where
// strings.HasPrefix(r.URL.Path, "/api/") failed when base_path was set, causing a page
// redirect instead of a 401 JSON response.
func TestBasePath_APIRequiresAuth(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	for _, path := range []string{"/api/nodes", "/api/profile"} {
		resp, err := client.Get(bpURL(path))
		if err != nil {
			t.Fatalf("GET %s%s: %v", basePath, path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s%s: expected non-200 for unauthenticated request, got 200", basePath, path)
		}
		// A redirect to /login (not /yaamon/login) indicates the base_path-aware
		// API path check is broken.
		if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
			loc := resp.Header.Get("Location")
			if !strings.HasPrefix(loc, basePath) {
				t.Errorf("%s%s: unauthenticated redirect Location %q does not include base_path %q", basePath, path, loc, basePath)
			}
		}
	}
}

// TestBasePath_AuthenticatedAPI verifies that authenticated API calls work under the base path.
func TestBasePath_AuthenticatedAPI(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.PostForm(bpURL("/login"), url.Values{
		"username": {"admin"},
		"password": {adminPassword},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()

	resp, err = client.Get(bpURL("/api/nodes"))
	if err != nil {
		t.Fatalf("GET %s/api/nodes: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("%s/api/nodes: expected 200 for authenticated request, got %d", basePath, resp.StatusCode)
	}
}

// TestBasePath_DashboardServesPage verifies that the dashboard page renders under the base path.
func TestBasePath_DashboardServesPage(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := loginBP(t, "admin", adminPassword)
	resp, err := client.Get(bpURL("/dashboard"))
	if err != nil {
		t.Fatalf("GET %s/dashboard: %v", basePath, err)
	}
	resp.Body.Close()
	// May redirect to a specific node or show the dashboard — either is fine.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Errorf("%s/dashboard: expected 200 or 302, got %d", basePath, resp.StatusCode)
	}
	// If it redirected, the Location should still be under the base path.
	if resp.StatusCode == http.StatusFound {
		loc := resp.Header.Get("Location")
		if !strings.HasPrefix(loc, basePath) {
			t.Errorf("%s/dashboard redirect: Location %q does not start with %s", basePath, loc, basePath)
		}
	}
}

// TestBasePath_SetupRedirectsWhenUsersExist verifies that /setup redirects to login when users exist.
func TestBasePath_SetupRedirectsWhenUsersExist(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(bpURL("/setup"))
	if err != nil {
		t.Fatalf("GET %s/setup: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("%s/setup: expected 302, got %d", basePath, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, basePath+"/login") {
		t.Errorf("%s/setup redirect: expected Location to start with %s/login, got %q", basePath, basePath, loc)
	}
}

// TestBasePath_RootNotFound verifies that paths outside the base path return 404.
func TestBasePath_RootNotFound(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	for _, path := range []string{"/login", "/dashboard", "/api/nodes"} {
		resp, err := http.Get(basepathURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("%s: expected non-200 (should only be served under %s), got 200", path, basePath)
		}
	}
}

// TestBasePath_SSEEndpointReachable verifies that the SSE endpoint URL includes the base path.
func TestBasePath_SSEEndpointReachable(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := loginBP(t, "admin", adminPassword)

	resp, err := client.Get(bpURL("/api/nodes"))
	if err != nil {
		t.Fatalf("GET %s/api/nodes: %v", basePath, err)
	}
	var nodes []struct {
		ID int64 `json:"id"`
	}
	decodeJSON(t, resp, &nodes)
	if len(nodes) == 0 {
		t.Skip("no nodes seeded; skipping SSE endpoint test")
	}

	sseURL := bpURL(fmt.Sprintf("/sse/%d", nodes[0].ID))
	req, _ := http.NewRequest(http.MethodGet, sseURL, nil)
	req.Header.Set("Accept", "text/event-stream")
	sseClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	sseResp, err := sseClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", sseURL, err)
	}
	sseResp.Body.Close()
	if sseResp.StatusCode == http.StatusNotFound {
		t.Errorf("%s: SSE endpoint returned 404 — route not registered under base_path", sseURL)
	}
}

// --- Fresh server tests (no users seeded) ---

// TestBasePathFresh_SetupPageRendered verifies that the setup page renders (200) when no
// users exist. This catches the setupGuard redirect loop: when base_path is set,
// r.URL.Path is /yaamon/setup (not /setup), so the guard's exemption check must use
// s.url("/setup") to avoid redirecting /setup → /setup infinitely.
func TestBasePathFresh_SetupPageRendered(t *testing.T) {
	skipIfNoBPFresh(t)
	waitForBPFreshServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(bpFreshURL("/setup"))
	if err != nil {
		t.Fatalf("GET %s/setup: %v", basePath, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s/setup (no users): expected 200, got %d — possible redirect loop", basePath, resp.StatusCode)
	}
	if !strings.Contains(string(body), "setup") && !strings.Contains(string(body), "Setup") {
		t.Errorf("%s/setup: response body doesn't look like a setup page", basePath)
	}
}

// TestBasePathFresh_RootRedirectsToSetup verifies that the root redirects to setup (not bare /setup)
// when no users exist.
func TestBasePathFresh_RootRedirectsToSetup(t *testing.T) {
	skipIfNoBPFresh(t)
	waitForBPFreshServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(bpFreshURL("/"))
	if err != nil {
		t.Fatalf("GET %s/: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("%s/ (no users): expected 302, got %d", basePath, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, basePath+"/setup") {
		t.Errorf("%s/ redirect: expected Location to start with %s/setup, got %q — setupGuard may not include base_path", basePath, basePath, loc)
	}
}

// TestBasePathFresh_StaticAssetsBeforeSetup verifies that static assets load on the setup
// page (before any users exist). The CSS and JS must be accessible or the setup form
// is unusable.
func TestBasePathFresh_StaticAssetsBeforeSetup(t *testing.T) {
	skipIfNoBPFresh(t)
	waitForBPFreshServer(t)

	for _, asset := range []string{"/static/app.css", "/static/app.js", "/favicon.png"} {
		resp, err := http.Get(bpFreshURL(asset))
		if err != nil {
			t.Fatalf("GET %s%s: %v", basePath, asset, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s%s (no users): expected 200, got %d", basePath, asset, resp.StatusCode)
		}
		if len(body) == 0 {
			t.Errorf("%s%s (no users): response body is empty", basePath, asset)
		}
	}
}
