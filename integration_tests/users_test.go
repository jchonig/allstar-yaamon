//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"testing"
)

func TestUsersListContainsSeeded(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodGet, "/api/users", nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var users []struct {
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	decodeJSON(t, resp, &users)
	found := map[string]bool{}
	for _, u := range users {
		found[u.Username] = true
	}
	if !found["admin"] {
		t.Error("seeded admin user not in list")
	}
	if !found["viewer"] {
		t.Error("seeded viewer user not in list")
	}
}

func TestUsersCRUD(t *testing.T) {
	c := adminClient(t)
	username := fmt.Sprintf("testuser-%s", uniqueNum())

	// Create
	resp := do(t, c, http.MethodPost, "/api/users", map[string]any{
		"username":   username,
		"password":   "Passw0rd!",
		"permission": "readonly",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create user: expected 201, got %d", resp.StatusCode)
	}
	var created struct {
		ID         int64  `json:"id"`
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	decodeJSON(t, resp, &created)
	if created.ID == 0 {
		t.Fatal("created user has zero ID")
	}
	if created.Username != username {
		t.Errorf("username: expected %q, got %q", username, created.Username)
	}
	if created.Permission != "readonly" {
		t.Errorf("permission: expected readonly, got %q", created.Permission)
	}

	t.Cleanup(func() {
		r := do(t, c, http.MethodDelete, fmt.Sprintf("/api/users/%d", created.ID), nil)
		r.Body.Close()
	})

	// Update permission
	resp = do(t, c, http.MethodPut, fmt.Sprintf("/api/users/%d", created.ID), map[string]any{
		"permission": "readwrite",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("update user: expected 200, got %d", resp.StatusCode)
	}
	var updated struct{ Permission string `json:"permission"` }
	decodeJSON(t, resp, &updated)
	if updated.Permission != "readwrite" {
		t.Errorf("permission not updated, got %q", updated.Permission)
	}

	// Delete
	resp = do(t, c, http.MethodDelete, fmt.Sprintf("/api/users/%d", created.ID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete user: expected 204, got %d", resp.StatusCode)
	}
}

func TestUserCreateInvalidPermission(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodPost, "/api/users", map[string]any{
		"username":   "badperm",
		"password":   "password",
		"permission": "invalid",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCannotDeleteLastSuperuser(t *testing.T) {
	c := adminClient(t)

	// Find admin's ID.
	resp := do(t, c, http.MethodGet, "/api/users", nil)
	var users []struct {
		ID         int64  `json:"id"`
		Username   string `json:"username"`
		Permission string `json:"permission"`
	}
	decodeJSON(t, resp, &users)

	var adminID int64
	var superuserCount int
	for _, u := range users {
		if u.Permission == "superuser" {
			superuserCount++
			if u.Username == "admin" {
				adminID = u.ID
			}
		}
	}
	if superuserCount > 1 {
		t.Skip("more than one superuser present — last-superuser guard not testable")
	}
	if adminID == 0 {
		t.Fatal("admin user not found")
	}

	resp = do(t, c, http.MethodDelete, fmt.Sprintf("/api/users/%d", adminID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 Conflict deleting last superuser, got %d", resp.StatusCode)
	}
}

func TestViewerCannotListUsers(t *testing.T) {
	c := viewerClient(t)
	resp := do(t, c, http.MethodGet, "/api/users", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
}
