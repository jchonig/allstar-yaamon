//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"testing"
)

func TestNodesListContainsSeeded(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodGet, "/api/nodes", nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var nodes []struct {
		ID         int64  `json:"id"`
		NodeNumber string `json:"node_number"`
		Name       string `json:"name"`
	}
	decodeJSON(t, resp, &nodes)
	for _, n := range nodes {
		if n.NodeNumber == "99999" {
			return
		}
	}
	t.Error("seeded node 99999 not found in /api/nodes response")
}

func TestNodesCRUD(t *testing.T) {
	c := adminClient(t)
	num := uniqueNum()

	// Create
	resp := do(t, c, http.MethodPost, "/api/nodes", map[string]any{
		"name":        fmt.Sprintf("CRUD Test Node %s", num),
		"node_number": num,
		"ami_host":    "localhost",
		"ami_port":    5038,
		"ami_user":    "admin",
		"ami_pass":    "test",
		"enabled":     false,
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create node: expected 201, got %d", resp.StatusCode)
	}
	var created struct {
		ID         int64  `json:"id"`
		NodeNumber string `json:"node_number"`
		Name       string `json:"name"`
	}
	decodeJSON(t, resp, &created)
	if created.ID == 0 {
		t.Fatal("created node has zero ID")
	}
	if created.NodeNumber != num {
		t.Errorf("node_number: expected %q, got %q", num, created.NodeNumber)
	}

	// Clean up at the end regardless of test outcome.
	t.Cleanup(func() {
		r := do(t, c, http.MethodDelete, fmt.Sprintf("/api/nodes/%d", created.ID), nil)
		r.Body.Close()
	})

	// Update
	resp = do(t, c, http.MethodPut, fmt.Sprintf("/api/nodes/%d", created.ID), map[string]any{
		"name": fmt.Sprintf("Updated Node %s", num),
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("update node: expected 200, got %d", resp.StatusCode)
	}
	var updated struct{ Name string `json:"name"` }
	decodeJSON(t, resp, &updated)
	if updated.Name != fmt.Sprintf("Updated Node %s", num) {
		t.Errorf("name not updated, got %q", updated.Name)
	}

	// Delete
	resp = do(t, c, http.MethodDelete, fmt.Sprintf("/api/nodes/%d", created.ID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete node: expected 204, got %d", resp.StatusCode)
	}
}

func TestNodeCreateRequiresNameAndNumber(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodPost, "/api/nodes", map[string]any{
		"name": "Missing Number",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestNodeCreateRequiresAMIUser(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodPost, "/api/nodes", map[string]any{
		"name":        "No AMI User Node",
		"node_number": uniqueNum(),
		"ami_host":    "localhost",
		"ami_port":    5038,
		// ami_user intentionally omitted
		"ami_pass": "pass",
		"enabled":  false,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("create without ami_user: expected 400, got %d", resp.StatusCode)
	}
}

func TestNodeUpdatePreservesAMIUser(t *testing.T) {
	c := adminClient(t)
	num := uniqueNum()

	// Create with a known ami_user.
	resp := do(t, c, http.MethodPost, "/api/nodes", map[string]any{
		"name":        "AMI User Preserve Test",
		"node_number": num,
		"ami_host":    "localhost",
		"ami_port":    5038,
		"ami_user":    "testadmin",
		"ami_pass":    "testpass",
		"enabled":     false,
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID int64 `json:"id"` }
	decodeJSON(t, resp, &created)
	t.Cleanup(func() {
		r := do(t, c, http.MethodDelete, fmt.Sprintf("/api/nodes/%d", created.ID), nil)
		r.Body.Close()
	})

	// Update with only the name — ami_user should be preserved.
	resp = do(t, c, http.MethodPut, fmt.Sprintf("/api/nodes/%d", created.ID), map[string]any{
		"name": "AMI User Preserve Test Updated",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Fetch the secret to confirm ami_user is still in the DB via the node list.
	resp = do(t, c, http.MethodGet, "/api/nodes", nil)
	var nodes []struct {
		ID      int64  `json:"id"`
		AMIUser string `json:"ami_user"`
	}
	decodeJSON(t, resp, &nodes)
	for _, n := range nodes {
		if n.ID == created.ID {
			if n.AMIUser != "testadmin" {
				t.Errorf("ami_user after partial update = %q, want testadmin", n.AMIUser)
			}
			return
		}
	}
	t.Error("updated node not found in /api/nodes list")
}

func TestNodeSecretReturnsPassword(t *testing.T) {
	c := adminClient(t)
	num := uniqueNum()

	resp := do(t, c, http.MethodPost, "/api/nodes", map[string]any{
		"name":        "Secret Test Node",
		"node_number": num,
		"ami_host":    "localhost",
		"ami_port":    5038,
		"ami_user":    "admin",
		"ami_pass":    "s3cr3tP@ss",
		"enabled":     false,
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created struct{ ID int64 `json:"id"` }
	decodeJSON(t, resp, &created)
	t.Cleanup(func() {
		r := do(t, c, http.MethodDelete, fmt.Sprintf("/api/nodes/%d", created.ID), nil)
		r.Body.Close()
	})

	resp = do(t, c, http.MethodGet, fmt.Sprintf("/api/nodes/%d/secret", created.ID), nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("secret: expected 200, got %d", resp.StatusCode)
	}
	var result struct{ Secret string `json:"secret"` }
	decodeJSON(t, resp, &result)
	if result.Secret != "s3cr3tP@ss" {
		t.Errorf("secret = %q, want s3cr3tP@ss", result.Secret)
	}
}

func TestNodeSecretRequiresAdmin(t *testing.T) {
	c := viewerClient(t)
	nid := seedNodeID(t)
	resp := do(t, c, http.MethodGet, fmt.Sprintf("/api/nodes/%d/secret", nid), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer accessing /secret: expected 403, got %d", resp.StatusCode)
	}
}

func TestConnectionsEndpointReturnsJSON(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)

	// Request connections for a node number — may not be in cache, but must return valid JSON.
	resp := do(t, c, http.MethodGet, fmt.Sprintf("/api/nodes/%d/connections/99999", nid), nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		NodeNumber  string `json:"node_number"`
		Connections []struct {
			NodeNumber string `json:"node_number"`
			Web        bool   `json:"web"`
		} `json:"connections"`
		BubbleChartURL string `json:"bubble_chart_url"`
	}
	decodeJSON(t, resp, &result)
	if result.NodeNumber != "99999" {
		t.Errorf("node_number = %q, want 99999", result.NodeNumber)
	}
	if result.BubbleChartURL == "" {
		t.Error("bubble_chart_url should not be empty")
	}
	// connections may be empty (no cache) but must be present as an array, not null.
	if result.Connections == nil {
		t.Error("connections field should be an array, not null")
	}
}

func TestNodeDeleteNotFound(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodDelete, "/api/nodes/999999999", nil)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("deleting nonexistent node should not return 200")
	}
}
