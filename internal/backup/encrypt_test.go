package backup

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plain := []byte("hello, world — this is a test payload")
	ct, err := encrypt(plain, "correct-passphrase")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ct) <= len(plain) {
		t.Errorf("ciphertext (%d) should be longer than plaintext (%d)", len(ct), len(plain))
	}

	got, err := decrypt(ct, "correct-passphrase")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plain)
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	ct, _ := encrypt([]byte("secret data"), "right-pass")
	if _, err := decrypt(ct, "wrong-pass"); err == nil {
		t.Error("expected error decrypting with wrong passphrase")
	}
}

func TestDecryptTooShort(t *testing.T) {
	if _, err := decrypt([]byte("short"), "pass"); err == nil {
		t.Error("expected error for data shorter than salt+nonce")
	}
}

func TestEncryptProducesDistinctCiphertexts(t *testing.T) {
	// Two encryptions of the same plaintext must produce different ciphertexts
	// because each call generates a fresh random salt+nonce.
	plain := []byte("same plaintext")
	ct1, _ := encrypt(plain, "pass")
	ct2, _ := encrypt(plain, "pass")
	if bytes.Equal(ct1, ct2) {
		t.Error("two encryptions of the same data should not be equal")
	}
}

func TestEncryptDecryptEmptyPayload(t *testing.T) {
	ct, err := encrypt([]byte{}, "passphrase")
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}
	got, err := decrypt(ct, "passphrase")
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", len(got))
	}
}
