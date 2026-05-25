package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"allstar-yaamon/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestCreateAndInspect_Unencrypted(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)

	data, manifest, err := Create(ctx, database, "test-v1.0", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if manifest.Encrypted {
		t.Error("manifest.Encrypted should be false")
	}
	if manifest.AppVersion != "test-v1.0" {
		t.Errorf("AppVersion = %q", manifest.AppVersion)
	}
	if manifest.Format != "owbackup" {
		t.Errorf("Format = %q", manifest.Format)
	}
	if manifest.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}

	// Magic bytes.
	if string(data[:4]) != "YAAM" {
		t.Errorf("bad magic: %q", string(data[:4]))
	}
	// Encrypted flag must be clear.
	if data[5]&flagEncrypted != 0 {
		t.Error("encrypted flag should not be set")
	}

	// Inspect should return the same manifest.
	m2, err := Inspect(data)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if m2.Encrypted != manifest.Encrypted {
		t.Errorf("Inspect.Encrypted = %v, want %v", m2.Encrypted, manifest.Encrypted)
	}
	if m2.AppVersion != manifest.AppVersion {
		t.Errorf("Inspect.AppVersion = %q", m2.AppVersion)
	}
}

func TestCreateAndInspect_Encrypted(t *testing.T) {
	ctx := context.Background()
	database := openTestDB(t)

	data, manifest, err := Create(ctx, database, "dev", CreateOptions{
		Passphrase: "my-secret-passphrase",
	})
	if err != nil {
		t.Fatalf("Create encrypted: %v", err)
	}
	if !manifest.Encrypted {
		t.Error("manifest.Encrypted should be true")
	}
	if data[5]&flagEncrypted == 0 {
		t.Error("encrypted flag not set in file header")
	}

	// Inspect works without passphrase (manifest is always plaintext).
	m2, err := Inspect(data)
	if err != nil {
		t.Fatalf("Inspect encrypted: %v", err)
	}
	if !m2.Encrypted {
		t.Error("Inspect.Encrypted should be true")
	}
}

func TestInspect_BadMagic(t *testing.T) {
	if _, err := Inspect([]byte("NOTABACKUP")); err == nil {
		t.Error("expected error for bad magic")
	}
}

func TestInspect_TooShort(t *testing.T) {
	if _, err := Inspect([]byte("YAA")); err == nil {
		t.Error("expected error for short data")
	}
}

func TestCreateAndRestore_Roundtrip(t *testing.T) {
	ctx := context.Background()

	// Create source DB with a node.
	srcDB := openTestDB(t)
	if _, err := srcDB.CreateNode(ctx, db.Node{
		Name: "Backup Test Node", NodeNumber: "55555",
		AMIHost: "localhost", AMIPort: 5038, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// Create a backup.
	data, _, err := Create(ctx, srcDB, "v1", CreateOptions{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Create a separate target DB to restore into.
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "restore.db")
	destDB, err := db.Open(destPath)
	if err != nil {
		t.Fatalf("open dest db: %v", err)
	}
	defer destDB.Close()

	preRestorePath, err := Restore(ctx, destDB, "v1", data, RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Pre-restore backup file must exist.
	if _, err := os.Stat(preRestorePath); err != nil {
		t.Errorf("pre-restore backup not found: %v", err)
	}
	t.Cleanup(func() { os.Remove(preRestorePath) })

	// Restored DB file must exist and be openable.
	restoredDB, err := db.Open(destPath)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer restoredDB.Close()

	nodes, err := restoredDB.ListNodes(ctx)
	if err != nil {
		t.Fatalf("list nodes from restored db: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeNumber != "55555" {
		t.Errorf("unexpected nodes after restore: %+v", nodes)
	}
}

func TestCreateAndRestore_WithPassphrase(t *testing.T) {
	ctx := context.Background()
	srcDB := openTestDB(t)

	data, _, err := Create(ctx, srcDB, "v1", CreateOptions{Passphrase: "secret"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	destDir := t.TempDir()
	destDB, _ := db.Open(filepath.Join(destDir, "restore.db"))
	defer destDB.Close()

	// Wrong passphrase must fail.
	if _, err := Restore(ctx, destDB, "v1", data, RestoreOptions{Passphrase: "wrong"}); err == nil {
		t.Error("expected error restoring with wrong passphrase")
	}

	// Correct passphrase must succeed.
	preRestorePath, err := Restore(ctx, destDB, "v1", data, RestoreOptions{Passphrase: "secret"})
	if err != nil {
		t.Fatalf("Restore with correct passphrase: %v", err)
	}
	os.Remove(preRestorePath)
}

func TestSplitFile_ManifestContentsCount(t *testing.T) {
	ctx := context.Background()
	srcDB := openTestDB(t)

	// Seed data.
	n, _ := srcDB.CreateNode(ctx, db.Node{Name: "N", NodeNumber: "11111", Enabled: true})
	srcDB.CreateFavorite(ctx, db.Favorite{NodeID: n.ID, NodeNumber: "22222"}) //nolint:errcheck
	srcDB.CreateFavorite(ctx, db.Favorite{NodeID: n.ID, NodeNumber: "33333"}) //nolint:errcheck

	data, manifest, _ := Create(ctx, srcDB, "v1", CreateOptions{})

	if manifest.Contents.Nodes != 1 {
		t.Errorf("Contents.Nodes = %d, want 1", manifest.Contents.Nodes)
	}
	if manifest.Contents.Favorites != 2 {
		t.Errorf("Contents.Favorites = %d, want 2", manifest.Contents.Favorites)
	}

	// Verify Inspect reads the same counts without decryption.
	m2, _ := Inspect(data)
	if m2.Contents.Nodes != manifest.Contents.Nodes {
		t.Errorf("Inspect Contents.Nodes = %d", m2.Contents.Nodes)
	}
}
