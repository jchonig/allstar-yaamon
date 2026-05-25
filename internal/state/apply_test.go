package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"allstar-yaamon/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// TestLoadEnvSubstitution verifies that $VAR references are resolved.
func TestLoadEnvSubstitution(t *testing.T) {
	t.Setenv("TEST_PASS", "secret123")

	content := `
users:
  - username: admin
    permission: superuser
    password: $TEST_PASS
`
	f := writeTempYAML(t, content)
	sf, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if sf.Users[0].Password != "secret123" {
		t.Errorf("password = %q, want secret123", sf.Users[0].Password)
	}
}

// TestLoadMissingEnv verifies that an unset env var is a fatal error.
func TestLoadMissingEnv(t *testing.T) {
	os.Unsetenv("DEFINITELY_UNSET_VAR_XYZ")
	content := `
users:
  - username: admin
    permission: superuser
    password: $DEFINITELY_UNSET_VAR_XYZ
`
	f := writeTempYAML(t, content)
	_, err := Load(f)
	if err == nil {
		t.Fatal("expected error for missing env var, got nil")
	}
}

// TestApplyUsersCreateAndUpdate exercises user creation and permission update.
func TestApplyUsersCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	sf := &StateFile{
		Users: []UserSpec{
			{Username: "alice", Permission: "superuser", Password: "password1234"},
			{Username: "bob", Permission: "readonly", Password: "password5678"},
		},
	}

	report, err := Apply(ctx, database, sf, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.UsersCreated != 2 {
		t.Errorf("UsersCreated = %d, want 2", report.UsersCreated)
	}

	// Change alice's permission.
	sf.Users[0].Permission = "admin"
	report, err = Apply(ctx, database, sf, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply (update): %v", err)
	}
	if report.UsersUpdated != 1 {
		t.Errorf("UsersUpdated = %d, want 1", report.UsersUpdated)
	}
}

// TestApplyUsersPurge verifies unlisted users are deleted when purge.users=true.
func TestApplyUsersPurge(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	// Seed two users.
	sf := &StateFile{
		Users: []UserSpec{
			{Username: "alice", Permission: "superuser", Password: "password1234"},
			{Username: "bob", Permission: "readonly", Password: "password5678"},
		},
	}
	if _, err := Apply(ctx, database, sf, ApplyOptions{}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Apply with only alice; bob should be purged.
	sf2 := &StateFile{
		Purge: PurgePolicy{Users: true},
		Users: []UserSpec{
			{Username: "alice", Permission: "superuser", Password: "password1234"},
		},
	}
	report, err := Apply(ctx, database, sf2, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply (purge): %v", err)
	}
	if report.UsersDeleted != 1 {
		t.Errorf("UsersDeleted = %d, want 1", report.UsersDeleted)
	}
}

// TestApplyNodesCreateAndFavorites exercises node+favorite creation.
func TestApplyNodesCreateAndFavorites(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	sf := &StateFile{
		Purge: PurgePolicy{Favorites: true},
		Users: []UserSpec{
			{Username: "admin", Permission: "superuser", Password: "password1234"},
		},
		Nodes: []NodeSpec{
			{
				Name: "Test Node", NodeNumber: "99999",
				AMIHost: "localhost", AMIPort: 5038,
				AMIUser: "admin", AMIPass: "pass",
				Enabled: true,
				Favorites: []FavoriteSpec{
					{NodeNumber: "12345", Callsign: "W1AW", Description: "ARRL HQ"},
					{NodeNumber: "27339", Callsign: "N0CALL", Description: "Example"},
				},
			},
		},
	}

	report, err := Apply(ctx, database, sf, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.NodesCreated != 1 {
		t.Errorf("NodesCreated = %d, want 1", report.NodesCreated)
	}
	if report.FavsCreated != 2 {
		t.Errorf("FavsCreated = %d, want 2", report.FavsCreated)
	}

	// Remove one favorite; with purge.favorites=true it should be deleted.
	sf.Nodes[0].Favorites = sf.Nodes[0].Favorites[:1]
	report, err = Apply(ctx, database, sf, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply (remove fav): %v", err)
	}
	if report.FavsDeleted != 1 {
		t.Errorf("FavsDeleted = %d, want 1", report.FavsDeleted)
	}
}

// TestApplyDryRun verifies nothing is written to the DB in dry-run mode.
func TestApplyDryRun(t *testing.T) {
	ctx := context.Background()
	database := newTestDB(t)

	sf := &StateFile{
		Users: []UserSpec{
			{Username: "admin", Permission: "superuser", Password: "password1234"},
		},
		Nodes: []NodeSpec{
			{Name: "Test", NodeNumber: "11111", Enabled: true},
		},
	}

	report, err := Apply(ctx, database, sf, ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	if report.UsersCreated != 1 || report.NodesCreated != 1 {
		t.Errorf("dry-run report = %+v, expected 1 user + 1 node created", report)
	}

	// Nothing should actually be in the DB.
	users, _ := database.ListUsers(ctx)
	if len(users) != 0 {
		t.Errorf("dry-run wrote %d users to DB, want 0", len(users))
	}
	nodes, _ := database.ListNodes(ctx)
	if len(nodes) != 0 {
		t.Errorf("dry-run wrote %d nodes to DB, want 0", len(nodes))
	}
}

// TestResolveEnvDoubleDollar verifies that $$ is an escape for a literal $.
func TestResolveEnvDoubleDollar(t *testing.T) {
	content := `
nodes:
  - name: Test
    node_number: "11111"
    ami_pass: $$secret$dollar
`
	f := writeTempYAML(t, content)
	sf, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := sf.Nodes[0].AMIPass; got != "$secret$dollar" {
		t.Errorf("ami_pass = %q, want %q", got, "$secret$dollar")
	}
}

// TestResolveEnvPlainValue verifies that values without $ are returned unchanged.
func TestResolveEnvPlainValue(t *testing.T) {
	content := `
nodes:
  - name: Test
    node_number: "22222"
    ami_pass: plainpassword
`
	f := writeTempYAML(t, content)
	sf, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := sf.Nodes[0].AMIPass; got != "plainpassword" {
		t.Errorf("ami_pass = %q, want %q", got, "plainpassword")
	}
}

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "state*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
