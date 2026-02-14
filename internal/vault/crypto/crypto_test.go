package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateSalt(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}
	if len(salt) != SaltLen {
		t.Errorf("salt length = %d, want %d", len(salt), SaltLen)
	}

	// Two salts should be different
	salt2, _ := GenerateSalt()
	if bytes.Equal(salt, salt2) {
		t.Error("two generated salts should not be identical")
	}
}

func TestDeriveKey(t *testing.T) {
	salt, _ := GenerateSalt()

	key, err := DeriveKey("test-password", salt)
	if err != nil {
		t.Fatalf("DeriveKey() error: %v", err)
	}
	if len(key) != 32 {
		t.Errorf("key length = %d, want 32", len(key))
	}

	// Same password + salt = same key
	key2, _ := DeriveKey("test-password", salt)
	if !bytes.Equal(key, key2) {
		t.Error("same password and salt should produce same key")
	}

	// Different password = different key
	key3, _ := DeriveKey("different-password", salt)
	if bytes.Equal(key, key3) {
		t.Error("different passwords should produce different keys")
	}

	// Different salt = different key
	salt2, _ := GenerateSalt()
	key4, _ := DeriveKey("test-password", salt2)
	if bytes.Equal(key, key4) {
		t.Error("different salts should produce different keys")
	}
}

func TestDeriveKeyBadSalt(t *testing.T) {
	_, err := DeriveKey("password", []byte("short"))
	if err == nil {
		t.Error("DeriveKey() with short salt should error")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("hello, envsafe!")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	// Ciphertext should be longer than plaintext (nonce + tag)
	if len(ciphertext) <= len(plaintext) {
		t.Error("ciphertext should be longer than plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptEmpty(t *testing.T) {
	key := make([]byte, 32)
	plaintext := []byte{}

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error: %v", err)
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypt() = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1

	ciphertext, _ := Encrypt(key1, []byte("secret"))

	_, err := Decrypt(key2, ciphertext)
	if err == nil {
		t.Error("Decrypt() with wrong key should error")
	}
}

func TestDecryptTruncated(t *testing.T) {
	key := make([]byte, 32)

	_, err := Decrypt(key, []byte("short"))
	if err == nil {
		t.Error("Decrypt() with truncated data should error")
	}
}

func TestEncryptBadKeyLen(t *testing.T) {
	_, err := Encrypt([]byte("short"), []byte("data"))
	if err == nil {
		t.Error("Encrypt() with bad key length should error")
	}
}

func TestDecryptBadKeyLen(t *testing.T) {
	_, err := Decrypt([]byte("short"), make([]byte, 100))
	if err == nil {
		t.Error("Decrypt() with bad key length should error")
	}
}

func TestRoundTrip(t *testing.T) {
	// Full round-trip: password → key derivation → encrypt → decrypt
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatal(err)
	}

	key, err := DeriveKey("my-master-password", salt)
	if err != nil {
		t.Fatal(err)
	}

	secrets := []byte(`{"development":{"DB_URL":"postgres://localhost/mydb","API_KEY":"sk-test-123"}}`)

	ciphertext, err := Encrypt(key, secrets)
	if err != nil {
		t.Fatal(err)
	}

	// Re-derive key from same password + salt
	key2, _ := DeriveKey("my-master-password", salt)
	decrypted, err := Decrypt(key2, ciphertext)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(secrets, decrypted) {
		t.Errorf("round-trip failed: got %q, want %q", decrypted, secrets)
	}
}
