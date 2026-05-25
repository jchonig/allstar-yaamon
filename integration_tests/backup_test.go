//go:build integration

package integration_tests

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBackupCreate(t *testing.T) {
	c := adminClient(t)

	// Unencrypted backup
	resp := do(t, c, http.MethodPost, "/api/backup", map[string]any{
		"passphrase": "",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("create backup: expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/octet-stream" {
		t.Errorf("Content-Type: expected application/octet-stream, got %q", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, ".owbackup") {
		t.Errorf("Content-Disposition: expected .owbackup filename, got %q", cd)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read backup body: %v", err)
	}
	if len(data) < 16 {
		t.Errorf("backup too small: %d bytes", len(data))
	}
	// Check magic bytes.
	if string(data[:4]) != "YAAM" {
		t.Errorf("expected magic YAAM, got %q", string(data[:4]))
	}
}

func TestBackupCreateEncrypted(t *testing.T) {
	c := adminClient(t)
	resp := do(t, c, http.MethodPost, "/api/backup", map[string]any{
		"passphrase": "test-secret-123",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("create encrypted backup: expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(data[:4]) != "YAAM" {
		t.Fatalf("bad magic: %q", string(data[:4]))
	}
	// Flag byte should have the encrypted bit set.
	if data[5]&0x01 == 0 {
		t.Error("encrypted flag not set in backup file header")
	}
}

func TestBackupInspect(t *testing.T) {
	c := adminClient(t)

	// First create a backup to inspect.
	resp := do(t, c, http.MethodPost, "/api/backup", map[string]any{"passphrase": ""})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("create: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	// Inspect it.
	resp = doRaw(t, c, http.MethodPost, "/api/backup/inspect",
		"application/octet-stream", data, nil)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("inspect: expected 200, got %d", resp.StatusCode)
	}
	var manifest struct {
		Format        string `json:"format"`
		FormatVersion int    `json:"format_version"`
		AppVersion    string `json:"app_version"`
		Encrypted     bool   `json:"encrypted"`
		Contents      struct {
			Nodes int `json:"nodes"`
		} `json:"contents"`
	}
	decodeJSON(t, resp, &manifest)
	if manifest.Format != "owbackup" {
		t.Errorf("format: expected owbackup, got %q", manifest.Format)
	}
	if manifest.FormatVersion != 1 {
		t.Errorf("format_version: expected 1, got %d", manifest.FormatVersion)
	}
	if manifest.Encrypted {
		t.Error("unencrypted backup reported as encrypted")
	}
	if manifest.Contents.Nodes == 0 {
		t.Error("expected at least 1 node in backup contents")
	}
}

func TestBackupInspectInvalidFile(t *testing.T) {
	c := adminClient(t)
	resp := doRaw(t, c, http.MethodPost, "/api/backup/inspect",
		"application/octet-stream", []byte("not a backup file"), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBackupViewerBlocked(t *testing.T) {
	c := viewerClient(t)
	resp := do(t, c, http.MethodPost, "/api/backup", map[string]any{"passphrase": ""})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("viewer backup: expected 403, got %d", resp.StatusCode)
	}
}
