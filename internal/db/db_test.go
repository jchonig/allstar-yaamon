package db

import (
	"context"
	"os"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "yaamon-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := Open(f.Name())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigration(t *testing.T) {
	db := openTestDB(t)
	var v int
	if err := db.sql.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&v); err != nil {
		t.Fatalf("schema_version: %v", err)
	}
	if v != 1 {
		t.Errorf("expected schema version 1, got %d", v)
	}
}

func TestUserCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Create
	u, err := db.CreateUser(ctx, "alice", "$2a$12$fakehash", PermAdmin)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Error("expected non-zero ID")
	}

	// GetUser
	got, err := db.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Username != "alice" || got.Permission != PermAdmin {
		t.Errorf("unexpected user: %+v", got)
	}

	// NotFound
	if _, err := db.GetUser(ctx, "nobody"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// UpdateUserPermission
	if err := db.UpdateUserPermission(ctx, u.ID, PermReadOnly); err != nil {
		t.Fatalf("UpdateUserPermission: %v", err)
	}
	got, _ = db.GetUser(ctx, "alice")
	if got.Permission != PermReadOnly {
		t.Errorf("expected readonly, got %s", got.Permission)
	}

	// CountSuperusers
	n, _ := db.CountSuperusers(ctx)
	if n != 0 {
		t.Errorf("expected 0 superusers, got %d", n)
	}

	// DeleteUser
	if err := db.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	count, _ := db.CountUsers(ctx)
	if count != 0 {
		t.Errorf("expected 0 users, got %d", count)
	}
}

func TestConfigGetSet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Missing key → empty string, no error.
	val, err := db.GetConfig(ctx, "missing_key")
	if err != nil {
		t.Fatalf("GetConfig missing: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty, got %q", val)
	}

	// Set then get.
	if err := db.SetConfig(ctx, "mykey", "myvalue"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	val, err = db.GetConfig(ctx, "mykey")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "myvalue" {
		t.Errorf("expected myvalue, got %q", val)
	}

	// Upsert (overwrite).
	db.SetConfig(ctx, "mykey", "newvalue")
	val, _ = db.GetConfig(ctx, "mykey")
	if val != "newvalue" {
		t.Errorf("expected newvalue, got %q", val)
	}
}
