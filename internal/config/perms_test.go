package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %04o %s: %v", mode, path, err)
	}
	return path
}

func TestCheckFilePermissions_ConfigFile(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		mode    os.FileMode
		wantLen int
	}{
		{0600, 0}, // owner-only — no warning
		{0640, 0}, // group-readable — acceptable for config
		{0644, 1}, // world-readable — warn
		{0664, 1}, // world-readable — warn
		{0604, 1}, // world-readable — warn
	}

	for _, tc := range tests {
		path := writeTemp(t, dir, "config.yaml", tc.mode)
		cfg := &Config{configFile: path}
		warns := CheckFilePermissions(cfg, "")
		if len(warns) != tc.wantLen {
			t.Errorf("mode %04o: got %d warnings, want %d: %v", tc.mode, len(warns), tc.wantLen, warns)
		}
		os.Remove(path)
	}
}

func TestCheckFilePermissions_TLSKey(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		mode    os.FileMode
		wantLen int
	}{
		{0600, 0}, // owner-only — no warning
		{0640, 1}, // group-readable — warn for private key
		{0644, 1}, // world-readable — warn
		{0604, 1}, // world-readable — warn
	}

	for _, tc := range tests {
		path := writeTemp(t, dir, "key.pem", tc.mode)
		cfg := &Config{TLS: TLSConfig{Mode: "provided", KeyFile: path}}
		warns := CheckFilePermissions(cfg, "")
		if len(warns) != tc.wantLen {
			t.Errorf("TLS key mode %04o: got %d warnings, want %d: %v", tc.mode, len(warns), tc.wantLen, warns)
		}
		os.Remove(path)
	}
}

func TestCheckFilePermissions_StateFile(t *testing.T) {
	dir := t.TempDir()

	// 0640 state file — no warning (group-readable is acceptable)
	path640 := writeTemp(t, dir, "state-640.yaml", 0640)
	cfg := &Config{}
	warns := CheckFilePermissions(cfg, path640)
	if len(warns) != 0 {
		t.Errorf("0640 state file: unexpected warnings: %v", warns)
	}

	// 0644 state file — world-readable, warn
	path644 := writeTemp(t, dir, "state-644.yaml", 0644)
	warns = CheckFilePermissions(cfg, path644)
	if len(warns) != 1 {
		t.Errorf("0644 state file: got %d warnings, want 1: %v", len(warns), warns)
	}
}

func TestCheckFilePermissions_MissingFile(t *testing.T) {
	// Non-existent files should produce no warnings (file may be created later).
	cfg := &Config{
		configFile: "/nonexistent/config.yaml",
		TLS:        TLSConfig{Mode: "provided", KeyFile: "/nonexistent/key.pem"},
	}
	warns := CheckFilePermissions(cfg, "/nonexistent/state.yaml")
	if len(warns) != 0 {
		t.Errorf("missing files: unexpected warnings: %v", warns)
	}
}

func TestCheckFilePermissions_ACMECacheDir(t *testing.T) {
	dir := t.TempDir()

	// 0700 — no warning
	acmeDir := filepath.Join(dir, "acme")
	if err := os.Mkdir(acmeDir, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{TLS: TLSConfig{Mode: "acme", ACMECacheDir: acmeDir}}
	warns := CheckFilePermissions(cfg, "")
	if len(warns) != 0 {
		t.Errorf("0700 acme dir: unexpected warnings: %v", warns)
	}

	// 0755 — world-readable, warn
	if err := os.Chmod(acmeDir, 0755); err != nil {
		t.Fatal(err)
	}
	warns = CheckFilePermissions(cfg, "")
	if len(warns) != 1 {
		t.Errorf("0755 acme dir: got %d warnings, want 1: %v", len(warns), warns)
	}
}

func TestCheckFilePermissions_NoConfigFile(t *testing.T) {
	// Empty configFile (no config file found) should not warn.
	cfg := &Config{}
	warns := CheckFilePermissions(cfg, "")
	if len(warns) != 0 {
		t.Errorf("empty config: unexpected warnings: %v", warns)
	}
}
