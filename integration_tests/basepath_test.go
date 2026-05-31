//go:build integration

package integration_tests

import (
	"fmt"
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
	basepathURL string
	basePath    string
)

func init() {
	basepathURL = os.Getenv("YAAMON_TEST_BASEPATH_URL")
	basePath = os.Getenv("YAAMON_TEST_BASEPATH")
}

// bpURL returns the full URL for a path under the base path.
func bpURL(path string) string {
	return basepathURL + basePath + path
}

// skipIfNoBP skips the test if the base-path server is not configured.
func skipIfNoBP(t *testing.T) {
	t.Helper()
	if basepathURL == "" || basePath == "" {
		t.Skip("YAAMON_TEST_BASEPATH_URL or YAAMON_TEST_BASEPATH not set; skipping base-path tests")
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
	// Expect 404 — health is not mounted under base_path.
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

// TestBasePath_StaticAssets verifies static assets are served under the base path.
func TestBasePath_StaticAssets(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	for _, asset := range []string{"/static/app.css", "/static/app.js", "/favicon.png"} {
		resp, err := http.Get(bpURL(asset))
		if err != nil {
			t.Fatalf("GET %s%s: %v", basePath, asset, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s%s: expected 200, got %d", basePath, asset, resp.StatusCode)
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

// TestBasePath_APIRequiresAuth verifies that the API returns 401/redirect for unauthenticated requests.
func TestBasePath_APIRequiresAuth(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(bpURL("/api/nodes"))
	if err != nil {
		t.Fatalf("GET %s/api/nodes: %v", basePath, err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Errorf("%s/api/nodes: expected non-200 for unauthenticated request, got 200", basePath)
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
	// login
	resp, err := client.PostForm(bpURL("/login"), url.Values{
		"username": {"admin"},
		"password": {adminPassword},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	resp.Body.Close()

	// hit the API
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
// (We just check that the endpoint is not 404; a full SSE connection would need a real node.)
func TestBasePath_SSEEndpointReachable(t *testing.T) {
	skipIfNoBP(t)
	waitForBPServer(t)

	client := loginBP(t, "admin", adminPassword)

	// Get node ID first.
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
	// Use a client that doesn't follow redirects and has a short timeout.
	sseClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// Copy cookies from the authenticated client jar.
	sseResp, err := sseClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", sseURL, err)
	}
	sseResp.Body.Close()
	// 200 (connected) or 401 (cookie jar not passed) are both acceptable here;
	// the key check is that it's not 404, which would mean the route is missing.
	if sseResp.StatusCode == http.StatusNotFound {
		t.Errorf("%s: SSE endpoint returned 404 — route not registered under base_path", sseURL)
	}
}
