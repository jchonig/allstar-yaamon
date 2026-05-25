package auth

import "testing"

func TestHashAndCheck(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := CheckPassword(hash, "correct-password"); err != nil {
		t.Errorf("CheckPassword correct: %v", err)
	}
	if err := CheckPassword(hash, "wrong-password"); err == nil {
		t.Error("CheckPassword wrong: expected error")
	}
}

func TestHashPasswordTooShort(t *testing.T) {
	if _, err := HashPassword("short"); err == nil {
		t.Error("expected error for short password")
	}
}
