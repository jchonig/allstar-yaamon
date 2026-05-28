//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// --- Checklist item 1: All API endpoints enforce permission level ---

func TestSecurity_PermissionEnforcement(t *testing.T) {
	viewer := viewerClient(t)
	cases := []struct {
		method string
		path   string
		body   any
	}{
		// Admin-only writes
		{http.MethodPost, "/api/nodes", map[string]any{"name": "x", "node_number": "12345", "ami_user": "u"}},
		{http.MethodPut, "/api/nodes/1", map[string]any{"name": "x"}},
		{http.MethodDelete, "/api/nodes/1", nil},
		{http.MethodPost, "/api/users", map[string]any{"username": "x", "password": "aaaaaaaa", "permission": "readonly"}},
		{http.MethodPut, "/api/users/1", map[string]any{"permission": "readonly"}},
		{http.MethodDelete, "/api/users/1", nil},
		{http.MethodPost, "/api/backup", nil},
		// Readwrite-only writes
		{http.MethodPost, "/api/nodes/1/connect", map[string]any{"node_number": "12345"}},
		{http.MethodPost, "/api/nodes/1/disconnect", map[string]any{"node_number": "12345"}},
		{http.MethodPost, "/api/nodes/1/favorites", map[string]any{"node_number": "12345"}},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			resp := do(t, viewer, tc.method, tc.path, tc.body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("viewer %s %s: expected 403, got %d", tc.method, tc.path, resp.StatusCode)
			}
		})
	}
}

func TestSecurity_UnauthenticatedBlocked(t *testing.T) {
	protected := []string{
		"/dashboard", "/admin/nodes", "/admin/users", "/admin/backup",
		"/settings/favorites",
	}
	for _, path := range protected {
		t.Run(path, func(t *testing.T) {
			resp := get(t, path)
			resp.Body.Close()
			if resp.StatusCode != http.StatusSeeOther {
				t.Errorf("%s unauthenticated: expected 303, got %d", path, resp.StatusCode)
			}
		})
	}
}

// --- Checklist item 2: CSRF — SameSite=Strict + Origin check ---

func TestSecurity_SessionCookieSameSiteStrict(t *testing.T) {
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {"admin"},
		"password": {adminPassword},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: expected 303, got %d", resp.StatusCode)
	}

	var found bool
	for _, cookie := range resp.Cookies() {
		if cookie.Name != "yaamon_session" {
			continue
		}
		found = true
		if !cookie.HttpOnly {
			t.Error("session cookie: HttpOnly not set")
		}
		if cookie.SameSite != http.SameSiteStrictMode {
			t.Errorf("session cookie: SameSite = %v, want Strict", cookie.SameSite)
		}
	}
	if !found {
		t.Error("no session cookie in login response")
	}
}

func TestSecurity_CSRFOriginMismatchRejected(t *testing.T) {
	// A POST with a mismatched Origin header must be rejected with 403,
	// regardless of whether the session cookie is valid.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/login", strings.NewReader("username=admin&password=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("mismatched Origin: expected 403, got %d", resp.StatusCode)
	}
}

func TestSecurity_CSRFNoOriginAllowed(t *testing.T) {
	// Requests without an Origin header (API clients, curl) must pass through.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {"admin"},
		"password": {"wrongpassword"},
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	// Should get 401 (wrong password), not 403 (CSRF).
	if resp.StatusCode == http.StatusForbidden {
		t.Error("request without Origin header should not be rejected by CSRF check")
	}
}

// --- Checklist item 3: bcrypt cost factor 12 ---
// Verified in internal/auth/password_test.go (unit test, not integration).

// --- Checklist item 4: AMI passwords stored in DB, not config ---

func TestSecurity_AMIPasswordInDBNotExposed(t *testing.T) {
	// The node list endpoint must not return ami_pass in its JSON.
	c := adminClient(t)
	resp := do(t, c, http.MethodGet, "/api/nodes", nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// Decode as raw map to catch any field name for the password.
	var nodes []map[string]any
	decodeJSON(t, resp, &nodes)
	for _, n := range nodes {
		if _, ok := n["ami_pass"]; ok {
			t.Error("node list response must not include ami_pass")
		}
		if _, ok := n["password"]; ok {
			t.Error("node list response must not include password")
		}
	}
}

// --- Checklist item 5 & 6: TLS 1.2 minimum and HSTS ---
// TLS version is enforced in tls.Config (unit-level) and cannot be exercised
// against the HTTP-only test server. HSTS is only sent over HTTPS.
// These are verified by code review of internal/tls/server.go and
// internal/server/security.go.

// --- Checklist item 7: Login rate limiting ---

func TestSecurity_LoginRateLimit(t *testing.T) {
	// Use a dedicated test IP via X-Forwarded-For so the rate-limiter tracks
	// this synthetic address instead of 127.0.0.1, leaving the shared loopback
	// address unblocked for all other tests.
	const testIP = "192.0.2.99" // TEST-NET, reserved by RFC 5737

	doLogin := func(password string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, baseURL+"/login",
			strings.NewReader("username=admin&password="+url.QueryEscape(password)))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-Forwarded-For", testIP)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		return resp
	}

	// Fire loginMaxFailures+1 bad attempts.
	// The first loginMaxFailures should return 401; the next must return 429.
	const maxFailures = 5
	for i := 0; i < maxFailures; i++ {
		resp := doLogin(fmt.Sprintf("wrong-attempt-%d", i))
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("attempt %d: expected 401, got %d", i, resp.StatusCode)
		}
	}

	// This attempt should be rate-limited.
	resp := doLogin("wrong-rate-limited")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("after %d failures: expected 429, got %d", maxFailures, resp.StatusCode)
	}
}

// --- Checklist item 8: Session cookie HttpOnly + Secure + SameSite ---
// Covered by TestSecurity_SessionCookieSameSiteStrict above (HttpOnly + SameSite).
// Secure is only set when TLS is active; the test server uses plain HTTP.

// --- Checklist item 9: No AMI credentials in logs ---
// Not automatable via HTTP; enforced by code review of internal/ami/client.go.

// --- Checklist item 10: Node number input validation ---

func TestSecurity_NodeNumberValidation(t *testing.T) {
	c := adminClient(t)
	cases := []struct {
		nodeNumber string
		wantOK     bool
	}{
		{"12345", true},   // valid 5-digit
		{"1234", true},    // valid 4-digit (minimum)
		{"1234567890", true}, // valid 10-digit (maximum)
		{"123", false},    // too short
		{"12345678901", false}, // too long
		{"abcde", false},  // non-numeric
		{"1234x", false},  // mixed
		{"", false},       // empty (already caught by required check)
		{"12 34", false},  // spaces
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("node_number=%q", tc.nodeNumber), func(t *testing.T) {
			body := map[string]any{
				"name":        "Validation Test",
				"node_number": tc.nodeNumber,
				"ami_user":    "testuser",
				"ami_pass":    "testpass",
				"enabled":     false,
			}
			resp := do(t, c, http.MethodPost, "/api/nodes", body)
			defer resp.Body.Close()
			if tc.wantOK {
				if resp.StatusCode != http.StatusCreated {
					t.Errorf("node_number=%q: expected 201, got %d", tc.nodeNumber, resp.StatusCode)
					return
				}
				// Clean up the created node.
				var created struct{ ID int64 `json:"id"` }
				decodeJSON(t, resp, &created)
				if created.ID != 0 {
					r := do(t, c, http.MethodDelete, fmt.Sprintf("/api/nodes/%d", created.ID), nil)
					r.Body.Close()
				}
			} else {
				if resp.StatusCode != http.StatusBadRequest {
					t.Errorf("node_number=%q: expected 400, got %d", tc.nodeNumber, resp.StatusCode)
				}
			}
		})
	}
}
