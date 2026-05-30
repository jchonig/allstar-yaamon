package db

import (
	"context"
	"testing"
)

func TestSetAndGetTailscaleLogins(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, _ := db.CreateUser(ctx, "alice", "hash", PermReadOnly)

	if err := db.SetTailscaleLogins(ctx, u.ID, []string{"alice@ts", "alice@github"}); err != nil {
		t.Fatalf("SetTailscaleLogins: %v", err)
	}

	logins, err := db.GetTailscaleLogins(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetTailscaleLogins: %v", err)
	}
	if len(logins) != 2 {
		t.Fatalf("expected 2 logins, got %d: %v", len(logins), logins)
	}
	// returned sorted
	if logins[0] != "alice@github" || logins[1] != "alice@ts" {
		t.Errorf("unexpected order: %v", logins)
	}
}

func TestSetTailscaleLogins_Replaces(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "alice", "hash", PermReadOnly)

	_ = db.SetTailscaleLogins(ctx, u.ID, []string{"old@ts"})
	if err := db.SetTailscaleLogins(ctx, u.ID, []string{"new@ts"}); err != nil {
		t.Fatalf("SetTailscaleLogins: %v", err)
	}

	logins, _ := db.GetTailscaleLogins(ctx, u.ID)
	if len(logins) != 1 || logins[0] != "new@ts" {
		t.Errorf("expected [new@ts], got %v", logins)
	}
}

func TestSetTailscaleLogins_Empty(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	u, _ := db.CreateUser(ctx, "alice", "hash", PermReadOnly)

	_ = db.SetTailscaleLogins(ctx, u.ID, []string{"alice@ts"})
	if err := db.SetTailscaleLogins(ctx, u.ID, nil); err != nil {
		t.Fatalf("SetTailscaleLogins nil: %v", err)
	}

	logins, _ := db.GetTailscaleLogins(ctx, u.ID)
	if len(logins) != 0 {
		t.Errorf("expected empty, got %v", logins)
	}
}

func TestSetTailscaleLogins_RejectsCrossUserDuplicate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u1, _ := db.CreateUser(ctx, "alice", "hash", PermReadOnly)
	u2, _ := db.CreateUser(ctx, "bob", "hash", PermReadOnly)

	if err := db.SetTailscaleLogins(ctx, u1.ID, []string{"shared@ts"}); err != nil {
		t.Fatalf("SetTailscaleLogins u1: %v", err)
	}

	if err := db.SetTailscaleLogins(ctx, u2.ID, []string{"shared@ts"}); err == nil {
		t.Error("expected error assigning duplicate login to second user, got nil")
	}

	// u1 should still own it
	got, err := db.GetUserByTailscaleLogin(ctx, "shared@ts")
	if err != nil || got.ID != u1.ID {
		t.Errorf("expected u1 to still own login, got user=%v err=%v", got, err)
	}
}

func TestGetUserByTailscaleLogin(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, _ := db.CreateUser(ctx, "alice", "hash", PermReadOnly)
	_ = db.SetTailscaleLogins(ctx, u.ID, []string{"alice@ts"})

	got, err := db.GetUserByTailscaleLogin(ctx, "alice@ts")
	if err != nil {
		t.Fatalf("GetUserByTailscaleLogin: %v", err)
	}
	if got.ID != u.ID || got.Username != "alice" {
		t.Errorf("unexpected user: %+v", got)
	}

	if _, err := db.GetUserByTailscaleLogin(ctx, "nobody@ts"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTailscaleLogins_CascadeOnUserDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	u, _ := db.CreateUser(ctx, "alice", "hash", PermReadOnly)
	_ = db.SetTailscaleLogins(ctx, u.ID, []string{"alice@ts"})

	if err := db.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	if _, err := db.GetUserByTailscaleLogin(ctx, "alice@ts"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound after user deleted, got %v", err)
	}
}
