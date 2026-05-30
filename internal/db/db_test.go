package db

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
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
	want := migrations[len(migrations)-1].version
	if v != want {
		t.Errorf("expected schema version %d, got %d", want, v)
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

func TestSchemaVersion(t *testing.T) {
	db := openTestDB(t)
	v, err := db.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	want := migrations[len(migrations)-1].version
	if v != want {
		t.Errorf("expected schema version %d, got %d", want, v)
	}
}

func TestNodeCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Create
	n, err := db.CreateNode(ctx, Node{
		Name: "Test Node", NodeNumber: "99999",
		AMIHost: "localhost", AMIPort: 5038,
		AMIUser: "admin", AMIPass: "pass",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if n.ID == 0 {
		t.Error("expected non-zero ID")
	}

	// GetByID
	got, err := db.GetNodeByID(ctx, n.ID)
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if got.NodeNumber != "99999" {
		t.Errorf("NodeNumber = %q", got.NodeNumber)
	}

	// GetByNumber
	got, err = db.GetNodeByNumber(ctx, "99999")
	if err != nil {
		t.Fatalf("GetNodeByNumber: %v", err)
	}
	if got.ID != n.ID {
		t.Errorf("ID mismatch: %d vs %d", got.ID, n.ID)
	}

	// NotFound
	if _, err := db.GetNodeByID(ctx, 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// List
	nodes, err := db.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	// ListNodeNumbers
	nums, err := db.ListNodeNumbers(ctx)
	if err != nil {
		t.Fatalf("ListNodeNumbers: %v", err)
	}
	if len(nums) != 1 || nums[0] != "99999" {
		t.Errorf("ListNodeNumbers = %v", nums)
	}

	// Update
	updated := *n
	updated.Name = "Updated Node"
	updated.Enabled = false
	if err := db.UpdateNode(ctx, updated); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}
	got, _ = db.GetNodeByID(ctx, n.ID)
	if got.Name != "Updated Node" || got.Enabled {
		t.Errorf("update not applied: %+v", got)
	}

	// Delete
	if err := db.DeleteNode(ctx, n.ID); err != nil {
		t.Fatalf("DeleteNode: %v", err)
	}
	if _, err := db.GetNodeByID(ctx, n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestFavoriteCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Need a node first.
	node, _ := db.CreateNode(ctx, Node{Name: "N", NodeNumber: "11111", Enabled: true})

	// Create
	f, err := db.CreateFavorite(ctx, Favorite{
		NodeID:      node.ID,
		NodeNumber:  "22222",
		Callsign:    "W1AW",
		Description: "ARRL HQ",
		Location:    "Newington CT",
		GroupName:   "default",
	})
	if err != nil {
		t.Fatalf("CreateFavorite: %v", err)
	}
	if f.ID == 0 {
		t.Error("expected non-zero ID")
	}

	// List
	favs, err := db.ListFavoritesByNode(ctx, node.ID)
	if err != nil {
		t.Fatalf("ListFavoritesByNode: %v", err)
	}
	if len(favs) != 1 || favs[0].NodeNumber != "22222" {
		t.Errorf("unexpected list: %+v", favs)
	}

	// GetByNodeNumber
	got, err := db.GetFavoriteByNodeNumber(ctx, node.ID, "22222")
	if err != nil {
		t.Fatalf("GetFavoriteByNodeNumber: %v", err)
	}
	if got.Callsign != "W1AW" {
		t.Errorf("Callsign = %q", got.Callsign)
	}

	// Update
	updated := *f
	updated.Description = "Updated"
	if err := db.UpdateFavorite(ctx, updated); err != nil {
		t.Fatalf("UpdateFavorite: %v", err)
	}
	got, _ = db.GetFavoriteByNodeNumber(ctx, node.ID, "22222")
	if got.Description != "Updated" {
		t.Errorf("Description = %q", got.Description)
	}

	// Add a second favorite for DeleteFavoritesByNode test.
	db.CreateFavorite(ctx, Favorite{NodeID: node.ID, NodeNumber: "33333"}) //nolint:errcheck

	// DeleteFavorite (single)
	if err := db.DeleteFavorite(ctx, f.ID); err != nil {
		t.Fatalf("DeleteFavorite: %v", err)
	}
	favs, _ = db.ListFavoritesByNode(ctx, node.ID)
	if len(favs) != 1 {
		t.Errorf("expected 1 after single delete, got %d", len(favs))
	}

	// DeleteFavoritesByNode (all for node)
	if err := db.DeleteFavoritesByNode(ctx, node.ID); err != nil {
		t.Fatalf("DeleteFavoritesByNode: %v", err)
	}
	favs, _ = db.ListFavoritesByNode(ctx, node.ID)
	if len(favs) != 0 {
		t.Errorf("expected 0 after delete-by-node, got %d", len(favs))
	}
}

func TestStats(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Empty DB.
	s, err := db.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if s.Nodes != 0 || s.Favorites != 0 || s.Users != 0 {
		t.Errorf("expected all zeros on empty DB, got %+v", s)
	}

	// Add data.
	db.CreateUser(ctx, "u", "hash", PermReadOnly) //nolint:errcheck
	n, _ := db.CreateNode(ctx, Node{Name: "N", NodeNumber: "11111", Enabled: true})
	db.CreateFavorite(ctx, Favorite{NodeID: n.ID, NodeNumber: "22222"}) //nolint:errcheck
	db.CreateFavorite(ctx, Favorite{NodeID: n.ID, NodeNumber: "33333"}) //nolint:errcheck

	s, err = db.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats after seed: %v", err)
	}
	if s.Nodes != 1 {
		t.Errorf("Nodes = %d, want 1", s.Nodes)
	}
	if s.Favorites != 2 {
		t.Errorf("Favorites = %d, want 2", s.Favorites)
	}
	if s.Users != 1 {
		t.Errorf("Users = %d, want 1", s.Users)
	}
}

func TestSnapshot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// Seed a node so there's something to verify after restore.
	db.CreateNode(ctx, Node{Name: "Snap Node", NodeNumber: "55555", Enabled: true}) //nolint:errcheck

	destPath := filepath.Join(t.TempDir(), "snap.db")
	if err := db.Snapshot(ctx, destPath); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// File must exist and be a valid DB.
	if _, err := os.Stat(destPath); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}

	snap, err := Open(destPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()

	nodes, err := snap.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes from snapshot: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeNumber != "55555" {
		t.Errorf("snapshot nodes = %+v", nodes)
	}
}

func TestUpdateUserProfile(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, "bob", "hash", PermReadOnly)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.UpdateUserProfile(ctx, u.ID, "Bob Smith", "https://example.com/bob.jpg"); err != nil {
		t.Fatalf("UpdateUserProfile: %v", err)
	}

	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.FullName != "Bob Smith" {
		t.Errorf("FullName = %q, want %q", got.FullName, "Bob Smith")
	}
	if got.AvatarURL != "https://example.com/bob.jpg" {
		t.Errorf("AvatarURL = %q, want %q", got.AvatarURL, "https://example.com/bob.jpg")
	}
}

func TestUserProfileFieldsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, _ := db.CreateUser(ctx, "carol", "hash", PermAdmin)
	db.UpdateUserProfile(ctx, u.ID, "Carol Jones", "https://example.com/carol.png") //nolint:errcheck

	// GetUser
	got, err := db.GetUser(ctx, "carol")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.FullName != "Carol Jones" || got.AvatarURL != "https://example.com/carol.png" {
		t.Errorf("GetUser profile mismatch: %+v", got)
	}

	// GetUserByID
	got, err = db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.FullName != "Carol Jones" || got.AvatarURL != "https://example.com/carol.png" {
		t.Errorf("GetUserByID profile mismatch: %+v", got)
	}

	// ListUsers
	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 || users[0].FullName != "Carol Jones" || users[0].AvatarURL != "https://example.com/carol.png" {
		t.Errorf("ListUsers profile mismatch: %+v", users)
	}
}

func TestPermissionHelpers(t *testing.T) {
	if !ValidPermission(PermReadOnly) {
		t.Error("readonly should be valid")
	}
	if ValidPermission("root") {
		t.Error("root should be invalid")
	}

	// Hierarchy: superuser > admin > readwrite > readonly
	cases := []struct{ a, b string }{
		{PermSuperuser, PermAdmin},
		{PermAdmin, PermReadWrite},
		{PermReadWrite, PermReadOnly},
		{PermSuperuser, PermReadOnly},
	}
	for _, c := range cases {
		if !PermissionAtLeast(c.a, c.b) {
			t.Errorf("PermissionAtLeast(%q, %q) should be true", c.a, c.b)
		}
		if PermissionAtLeast(c.b, c.a) {
			t.Errorf("PermissionAtLeast(%q, %q) should be false", c.b, c.a)
		}
	}
}

func TestUpdateUserQRZ(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, "alice", "hash", PermReadOnly)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Set credentials.
	if err := db.UpdateUserQRZ(ctx, u.ID, "W1AW", "encryptedpass"); err != nil {
		t.Fatalf("UpdateUserQRZ set: %v", err)
	}
	got, _ := db.GetUserByID(ctx, u.ID)
	if got.QRZUsername != "W1AW" {
		t.Errorf("QRZUsername = %q, want %q", got.QRZUsername, "W1AW")
	}
	if got.QRZPasswordEnc != "encryptedpass" {
		t.Errorf("QRZPasswordEnc = %q, want %q", got.QRZPasswordEnc, "encryptedpass")
	}

	// Clear credentials.
	if err := db.UpdateUserQRZ(ctx, u.ID, "", ""); err != nil {
		t.Fatalf("UpdateUserQRZ clear: %v", err)
	}
	got, _ = db.GetUserByID(ctx, u.ID)
	if got.QRZUsername != "" || got.QRZPasswordEnc != "" {
		t.Errorf("credentials not cleared: %+v", got)
	}
}

func TestUpdateUserLookupSource(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, "bob", "hash", PermReadOnly)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	for _, src := range []string{"auto", "qrz", "callook", ""} {
		if err := db.UpdateUserLookupSource(ctx, u.ID, src); err != nil {
			t.Fatalf("UpdateUserLookupSource(%q): %v", src, err)
		}
		got, _ := db.GetUserByID(ctx, u.ID)
		if got.LookupSource != src {
			t.Errorf("LookupSource = %q, want %q", got.LookupSource, src)
		}
	}
}

func TestUserQRZFieldsInListUsers(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, _ := db.CreateUser(ctx, "carol", "hash", PermAdmin)
	db.UpdateUserQRZ(ctx, u.ID, "W1AW", "enc") //nolint:errcheck
	db.UpdateUserLookupSource(ctx, u.ID, "qrz") //nolint:errcheck

	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 || users[0].QRZUsername != "W1AW" || users[0].LookupSource != "qrz" {
		t.Errorf("ListUsers QRZ fields mismatch: %+v", users)
	}
}
