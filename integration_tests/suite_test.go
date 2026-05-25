//go:build integration

package integration_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"testing"
	"time"
)

var (
	baseURL        string
	adminPassword  string
	viewerPassword string
)

func TestMain(m *testing.M) {
	baseURL = os.Getenv("YAAMON_TEST_URL")
	if baseURL == "" {
		fmt.Fprintln(os.Stderr, "YAAMON_TEST_URL not set — run via 'make test-integration'")
		os.Exit(1)
	}
	adminPassword = os.Getenv("TEST_ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "testpassword"
	}
	viewerPassword = os.Getenv("TEST_VIEWER_PASSWORD")
	if viewerPassword == "" {
		viewerPassword = "viewerpassword"
	}
	if err := waitForServer(baseURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "server not ready: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitForServer(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server at %s not ready after %s", url, timeout)
}

// get performs an unauthenticated GET without following redirects.
func get(t *testing.T, path string) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

// login returns an *http.Client carrying an authenticated session cookie.
// The client does NOT follow redirects so callers can inspect Location headers.
func login(t *testing.T, username, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.PostForm(baseURL+"/login", url.Values{
		"username": {username},
		"password": {password},
	})
	if err != nil {
		t.Fatalf("login POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %q: expected 303, got %d", username, resp.StatusCode)
	}
	return client
}

func adminClient(t *testing.T) *http.Client {
	t.Helper()
	return login(t, "admin", adminPassword)
}

func viewerClient(t *testing.T) *http.Client {
	t.Helper()
	return login(t, "viewer", viewerPassword)
}

// do performs method+path with an optional JSON body using the given client.
func do(t *testing.T, client *http.Client, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	ct := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
		ct = "application/json"
	}
	req, err := http.NewRequest(method, baseURL+path, r)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// doRaw performs method+path with a raw body (non-JSON) using the given client.
func doRaw(t *testing.T, client *http.Client, method, path, contentType string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// decodeJSON decodes resp.Body into v and closes the body.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON (status %d): %v", resp.StatusCode, err)
	}
}

// seedNodeID returns the ID of the seeded Test Node (node_number "99999").
func seedNodeID(t *testing.T) int64 {
	t.Helper()
	c := adminClient(t)
	resp := do(t, c, http.MethodGet, "/api/nodes", nil)
	var nodes []struct {
		ID         int64  `json:"id"`
		NodeNumber string `json:"node_number"`
	}
	decodeJSON(t, resp, &nodes)
	for _, n := range nodes {
		if n.NodeNumber == "99999" {
			return n.ID
		}
	}
	t.Fatal("seeded node 99999 not found")
	return 0
}

// uniqueNum returns a unique short number string for tests that need one.
func uniqueNum() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()%90000+10000)
}
