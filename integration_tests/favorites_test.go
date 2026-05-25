//go:build integration

package integration_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFavoritesCRUD(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)
	path := fmt.Sprintf("/api/nodes/%d", nid)
	num := uniqueNum()

	// Create
	resp := do(t, c, http.MethodPost, path+"/favorites", map[string]any{
		"node_number": num,
		"callsign":    "W1AW",
		"description": "Integration test favorite",
		"location":    "Newington, CT",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create favorite: expected 201, got %d", resp.StatusCode)
	}
	var created struct {
		ID          int64  `json:"id"`
		NodeNumber  string `json:"node_number"`
		Description string `json:"description"`
	}
	decodeJSON(t, resp, &created)
	if created.ID == 0 {
		t.Fatal("created favorite has zero ID")
	}
	if created.NodeNumber != num {
		t.Errorf("node_number: expected %q, got %q", num, created.NodeNumber)
	}

	t.Cleanup(func() {
		r := do(t, c, http.MethodDelete, fmt.Sprintf("%s/favorites/%d", path, created.ID), nil)
		r.Body.Close()
	})

	// List — should include the new favorite
	resp = do(t, c, http.MethodGet, path+"/favorites", nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list favorites: expected 200, got %d", resp.StatusCode)
	}
	var favs []struct {
		ID         int64  `json:"id"`
		NodeNumber string `json:"node_number"`
	}
	decodeJSON(t, resp, &favs)
	found := false
	for _, f := range favs {
		if f.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created favorite not found in list")
	}

	// Update
	resp = do(t, c, http.MethodPut, fmt.Sprintf("%s/favorites/%d", path, created.ID), map[string]any{
		"node_number": num,
		"description": "Updated description",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("update favorite: expected 200, got %d", resp.StatusCode)
	}
	var updated struct{ Description string `json:"description"` }
	decodeJSON(t, resp, &updated)
	if updated.Description != "Updated description" {
		t.Errorf("description not updated, got %q", updated.Description)
	}

	// Delete
	resp = do(t, c, http.MethodDelete, fmt.Sprintf("%s/favorites/%d", path, created.ID), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete favorite: expected 204, got %d", resp.StatusCode)
	}
}

func TestFavoritesExportImportRoundtrip(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)
	path := fmt.Sprintf("/api/nodes/%d", nid)
	num := uniqueNum()

	// Create a favorite to export.
	resp := do(t, c, http.MethodPost, path+"/favorites", map[string]any{
		"node_number": num,
		"callsign":    "K1TTT",
		"description": "Export test",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: %d", resp.StatusCode)
	}
	var created struct{ ID int64 `json:"id"` }
	decodeJSON(t, resp, &created)
	t.Cleanup(func() {
		r := do(t, c, http.MethodDelete, fmt.Sprintf("%s/favorites/%d", path, created.ID), nil)
		r.Body.Close()
	})

	// Export as INI.
	resp = do(t, c, http.MethodGet, path+"/favorites/export", nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("export: expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("export: expected text/plain, got %q", ct)
	}

	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, ".ini") {
		t.Errorf("export: expected .ini in Content-Disposition, got %q", cd)
	}
}

func TestFavoritesImportPreviewAndImport(t *testing.T) {
	c := adminClient(t)
	nid := seedNodeID(t)
	path := fmt.Sprintf("/api/nodes/%d", nid)
	num := uniqueNum()

	// Build a minimal favorites.ini.
	ini := fmt.Sprintf("[node_%s]\ncmd[]=ilink,3,%s\n", num, num)

	// Preview
	resp := doRaw(t, c, http.MethodPost, path+"/favorites/import/preview",
		"text/plain", []byte(ini), nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("preview: expected 200, got %d", resp.StatusCode)
	}
	var preview struct {
		Total    int `json:"total"`
		WillAdd  int `json:"will_add"`
		WillSkip int `json:"will_skip"`
	}
	decodeJSON(t, resp, &preview)
	if preview.Total == 0 {
		t.Error("preview: expected at least 1 total")
	}

	// Import
	resp = doRaw(t, c, http.MethodPost, path+"/favorites/import",
		"text/plain", []byte(ini), nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("import: expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		Added   int `json:"added"`
		Skipped int `json:"skipped"`
	}
	decodeJSON(t, resp, &result)
	if result.Added == 0 {
		t.Error("import: expected at least 1 added")
	}

	// Clean up imported favorites by listing and deleting the one we just added.
	t.Cleanup(func() {
		r := do(t, c, http.MethodGet, path+"/favorites", nil)
		var favs []struct {
			ID         int64  `json:"id"`
			NodeNumber string `json:"node_number"`
		}
		decodeJSON(t, r, &favs)
		for _, f := range favs {
			if f.NodeNumber == num {
				r2 := do(t, c, http.MethodDelete, fmt.Sprintf("%s/favorites/%d", path, f.ID), nil)
				r2.Body.Close()
			}
		}
	})
}

func TestViewerCanListFavorites(t *testing.T) {
	c := viewerClient(t)
	nid := seedNodeID(t)
	resp := do(t, c, http.MethodGet, fmt.Sprintf("/api/nodes/%d/favorites", nid), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("viewer list favorites: expected 200, got %d", resp.StatusCode)
	}
}

func TestViewerCannotCreateFavorite(t *testing.T) {
	c := viewerClient(t)
	nid := seedNodeID(t)
	resp := do(t, c, http.MethodPost, fmt.Sprintf("/api/nodes/%d/favorites", nid), map[string]any{
		"node_number": "12345",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer create favorite: expected 403, got %d", resp.StatusCode)
	}
}
