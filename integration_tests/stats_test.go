//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestDashboardWithNodeID(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)

	resp := do(t, c, http.MethodGet, fmt.Sprintf("/dashboard/%d", nid), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("expected HTML, got %q", ct)
	}
}

func TestNodeStats(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)

	resp := do(t, c, http.MethodGet, fmt.Sprintf("/api/nodes/%d/stats", nid), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	// Returns a JSON object (may be empty {} when AMI is not connected).
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON, got %q", ct)
	}
}

func TestNodeTest(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)

	resp := do(t, c, http.MethodPost, fmt.Sprintf("/api/nodes/%d/test", nid), nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	decodeJSON(t, resp, &result)
	// AMI is not running in test environment — expect ok=false with an error.
	if result.OK {
		t.Log("AMI test unexpectedly succeeded")
	} else if result.Error == "" {
		t.Error("ok=false but no error message")
	}
}

func TestConnectReturnsJSON(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)

	resp := do(t, c, http.MethodPost, fmt.Sprintf("/api/nodes/%d/connect", nid), map[string]any{
		"target":    "12345",
		"exclusive": false,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	decodeJSON(t, resp, &result)
	// Without a real Asterisk server this will return ok=false.
	// We just verify the response is well-formed JSON, not a 500.
	if result.OK {
		t.Log("connect unexpectedly succeeded (real AMI?)")
	}
}

func TestDisconnectReturnsJSON(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)

	resp := do(t, c, http.MethodPost, fmt.Sprintf("/api/nodes/%d/disconnect", nid), map[string]any{
		"target":    "12345",
		"permanent": false,
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	decodeJSON(t, resp, &result)
	if result.OK {
		t.Log("disconnect unexpectedly succeeded")
	}
}

func TestViewerCannotConnect(t *testing.T) {
	c := viewerClient(t)
	nid := seedNodeID(t)

	resp := do(t, c, http.MethodPost, fmt.Sprintf("/api/nodes/%d/connect", nid), map[string]any{
		"target": "12345",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}
