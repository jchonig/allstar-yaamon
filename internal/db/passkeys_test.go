package db

import (
	"context"
	"testing"
	"time"
)

func TestGetOrSetWebAuthnID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, err := db.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// First call generates and persists a new ID.
	id1, err := db.GetOrSetWebAuthnID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetOrSetWebAuthnID (1st): %v", err)
	}
	if len(id1) != 64 {
		t.Errorf("want 64-byte ID, got %d bytes", len(id1))
	}

	// Second call returns the same value without regenerating.
	id2, err := db.GetOrSetWebAuthnID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetOrSetWebAuthnID (2nd): %v", err)
	}
	if string(id1) != string(id2) {
		t.Errorf("ID changed between calls: %x != %x", id1, id2)
	}

	// Different user gets a different ID.
	u2, _ := db.CreateUser(ctx, "bob", "$2a$12$fakehash", PermReadOnly)
	id3, err := db.GetOrSetWebAuthnID(ctx, u2.ID)
	if err != nil {
		t.Fatalf("GetOrSetWebAuthnID (bob): %v", err)
	}
	if string(id1) == string(id3) {
		t.Error("two users got the same WebAuthn ID")
	}
}

func TestCreateAndListCredentials(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	u, _ := database.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)

	// Empty list initially.
	creds, err := database.ListCredentials(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("want 0 credentials, got %d", len(creds))
	}

	// Create two credentials.
	c1, err := database.CreateCredential(ctx, u.ID, []byte("credid-1"), "Touch ID", `{"id":"credid-1"}`)
	if err != nil {
		t.Fatalf("CreateCredential 1: %v", err)
	}
	_, err = database.CreateCredential(ctx, u.ID, []byte("credid-2"), "YubiKey", `{"id":"credid-2"}`)
	if err != nil {
		t.Fatalf("CreateCredential 2: %v", err)
	}

	creds, err = database.ListCredentials(ctx, u.ID)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("want 2 credentials, got %d", len(creds))
	}
	if creds[0].Name != "Touch ID" || creds[1].Name != "YubiKey" {
		t.Errorf("unexpected credential names: %q, %q", creds[0].Name, creds[1].Name)
	}

	// CountCredentials.
	n, err := database.CountCredentials(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountCredentials: %v", err)
	}
	if n != 2 {
		t.Errorf("want count 2, got %d", n)
	}

	// GetCredential.
	got, err := database.GetCredential(ctx, c1.ID, u.ID)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if got.Name != "Touch ID" {
		t.Errorf("GetCredential name = %q, want Touch ID", got.Name)
	}

	// GetCredential: wrong user returns ErrNotFound.
	u2, _ := database.CreateUser(ctx, "bob", "$2a$12$fakehash", PermReadOnly)
	if _, err := database.GetCredential(ctx, c1.ID, u2.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetCredentialsByWebAuthnID(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	u, _ := database.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)
	waID, _ := database.GetOrSetWebAuthnID(ctx, u.ID)
	database.CreateCredential(ctx, u.ID, []byte("credid-1"), "Touch ID", `{"id":"credid-1"}`) //nolint:errcheck

	creds, userID, err := database.GetCredentialsByWebAuthnID(ctx, waID)
	if err != nil {
		t.Fatalf("GetCredentialsByWebAuthnID: %v", err)
	}
	if userID != u.ID {
		t.Errorf("userID = %d, want %d", userID, u.ID)
	}
	if len(creds) != 1 {
		t.Errorf("want 1 credential, got %d", len(creds))
	}

	// Unknown WebAuthn ID returns ErrNotFound.
	_, _, err = database.GetCredentialsByWebAuthnID(ctx, []byte("no-such-id"))
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRenameCredential(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	u, _ := database.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)
	c, _ := database.CreateCredential(ctx, u.ID, []byte("credid-1"), "Touch ID", `{}`)

	if err := database.RenameCredential(ctx, c.ID, u.ID, "MacBook Pro"); err != nil {
		t.Fatalf("RenameCredential: %v", err)
	}
	got, _ := database.GetCredential(ctx, c.ID, u.ID)
	if got.Name != "MacBook Pro" {
		t.Errorf("name = %q, want MacBook Pro", got.Name)
	}

	// Wrong user returns ErrNotFound.
	u2, _ := database.CreateUser(ctx, "bob", "$2a$12$fakehash", PermReadOnly)
	if err := database.RenameCredential(ctx, c.ID, u2.ID, "X"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateCredentialUsed(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	u, _ := database.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)
	database.CreateCredential(ctx, u.ID, []byte("credid-1"), "Touch ID", `{"v":1}`) //nolint:errcheck

	if err := database.UpdateCredentialUsed(ctx, []byte("credid-1"), `{"v":2}`); err != nil {
		t.Fatalf("UpdateCredentialUsed: %v", err)
	}
	creds, _ := database.ListCredentials(ctx, u.ID)
	if creds[0].CredentialJSON != `{"v":2}` {
		t.Errorf("JSON not updated: %s", creds[0].CredentialJSON)
	}
	if creds[0].LastUsedAt == nil {
		t.Error("LastUsedAt not set after UpdateCredentialUsed")
	}
}

func TestDeleteCredential(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	u, _ := database.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)
	c, _ := database.CreateCredential(ctx, u.ID, []byte("credid-1"), "Touch ID", `{}`)

	if err := database.DeleteCredential(ctx, c.ID, u.ID); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
	n, _ := database.CountCredentials(ctx, u.ID)
	if n != 0 {
		t.Errorf("want 0 after delete, got %d", n)
	}

	// Second delete returns ErrNotFound.
	if err := database.DeleteCredential(ctx, c.ID, u.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Cross-user delete is blocked.
	u2, _ := database.CreateUser(ctx, "bob", "$2a$12$fakehash", PermReadOnly)
	c2, _ := database.CreateCredential(ctx, u2.ID, []byte("credid-2"), "YubiKey", `{}`)
	if err := database.DeleteCredential(ctx, c2.ID, u.ID); err != ErrNotFound {
		t.Errorf("cross-user delete: expected ErrNotFound, got %v", err)
	}
}

func TestWebAuthnSessions(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	userID := int64(42)
	exp := time.Now().Add(5 * time.Minute)

	// Create and retrieve a login session.
	if err := database.CreateWebAuthnSession(ctx, "sess-login", "login", "localhost", "http://localhost", nil, `{"challenge":"abc"}`, exp); err != nil {
		t.Fatalf("CreateWebAuthnSession (login): %v", err)
	}
	s, err := database.GetAndDeleteWebAuthnSession(ctx, "sess-login")
	if err != nil {
		t.Fatalf("GetAndDeleteWebAuthnSession: %v", err)
	}
	if s.Ceremony != "login" || s.UserID != nil {
		t.Errorf("unexpected session: ceremony=%s userID=%v", s.Ceremony, s.UserID)
	}
	if s.RPID != "localhost" || s.Origin != "http://localhost" {
		t.Errorf("rpid=%q origin=%q, want localhost / http://localhost", s.RPID, s.Origin)
	}

	// Session is deleted on retrieval (single-use).
	if _, err := database.GetAndDeleteWebAuthnSession(ctx, "sess-login"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound on second get, got %v", err)
	}

	// Registration session carries a user ID.
	if err := database.CreateWebAuthnSession(ctx, "sess-reg", "registration", "localhost", "http://localhost", &userID, `{}`, exp); err != nil {
		t.Fatalf("CreateWebAuthnSession (registration): %v", err)
	}
	s, _ = database.GetAndDeleteWebAuthnSession(ctx, "sess-reg")
	if s.UserID == nil || *s.UserID != userID {
		t.Errorf("userID = %v, want %d", s.UserID, userID)
	}

	// Non-existent session returns ErrNotFound.
	if _, err := database.GetAndDeleteWebAuthnSession(ctx, "no-such-session"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPruneWebAuthnSessions(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Minute)
	future := time.Now().Add(5 * time.Minute)

	database.CreateWebAuthnSession(ctx, "expired", "login", "localhost", "http://localhost", nil, `{}`, past)  //nolint:errcheck
	database.CreateWebAuthnSession(ctx, "valid", "login", "localhost", "http://localhost", nil, `{}`, future) //nolint:errcheck

	if err := database.PruneWebAuthnSessions(ctx); err != nil {
		t.Fatalf("PruneWebAuthnSessions: %v", err)
	}

	if _, err := database.GetAndDeleteWebAuthnSession(ctx, "expired"); err != ErrNotFound {
		t.Errorf("expired session should have been pruned, got %v", err)
	}
	if _, err := database.GetAndDeleteWebAuthnSession(ctx, "valid"); err != nil {
		t.Errorf("valid session should survive prune, got %v", err)
	}
}

func TestCredentialsCascadeOnUserDelete(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	u, _ := database.CreateUser(ctx, "alice", "$2a$12$fakehash", PermReadOnly)
	database.CreateCredential(ctx, u.ID, []byte("credid-1"), "Touch ID", `{}`) //nolint:errcheck

	if err := database.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	// Credential should be gone via ON DELETE CASCADE.
	var n int
	database.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, u.ID).Scan(&n) //nolint:errcheck
	if n != 0 {
		t.Errorf("expected credentials deleted by cascade, got %d", n)
	}
}
