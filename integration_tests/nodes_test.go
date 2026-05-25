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

func TestNodeDeleteNotFound(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodDelete, "/api/nodes/999999999", nil)
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("deleting nonexistent node should not return 200")
	}
}
