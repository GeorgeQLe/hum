package sharing

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

func TestHKDFSaltNotNil(t *testing.T) {
	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	vaultKey := make([]byte, 32)
	if _, err := rand.Read(vaultKey); err != nil {
		t.Fatalf("generating vault key: %v", err)
	}

	enc1, err := EncryptVaultKeyForUser(vaultKey, recipient.PublicKey)
	if err != nil {
		t.Fatalf("EncryptVaultKeyForUser() #1 error: %v", err)
	}

	enc2, err := EncryptVaultKeyForUser(vaultKey, recipient.PublicKey)
	if err != nil {
		t.Fatalf("EncryptVaultKeyForUser() #2 error: %v", err)
	}

	if enc1.EncryptedKey == enc2.EncryptedKey {
		t.Error("encrypting the same vault key for the same recipient twice should produce different ciphertexts (ephemeral keys differ, HKDF salt includes ephemeral key)")
	}
}

func TestEphemeralKeyUniqueness(t *testing.T) {
	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	vaultKey := make([]byte, 32)
	if _, err := rand.Read(vaultKey); err != nil {
		t.Fatalf("generating vault key: %v", err)
	}

	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		enc, err := EncryptVaultKeyForUser(vaultKey, recipient.PublicKey)
		if err != nil {
			t.Fatalf("EncryptVaultKeyForUser() iteration %d error: %v", i, err)
		}
		if seen[enc.EphemeralPub] {
			t.Fatalf("duplicate ephemeral public key at iteration %d: %s", i, enc.EphemeralPub)
		}
		seen[enc.EphemeralPub] = true
	}
}

func TestLowOrderPointRejection(t *testing.T) {
	lowOrderPoints := [][KeySize]byte{
		// All-zeros key
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// Key = {1, 0, 0, ...}
		{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// p-1 = 2^255-20
		{0xec, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		// p = 2^255-19
		{0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		// p+1
		{0xee, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
	}

	privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	for i, point := range lowOrderPoints {
		_, err := ComputeSharedSecret(privKey.PrivateKey, point)
		if err == nil {
			t.Errorf("low-order point #%d: ComputeSharedSecret should have returned an error", i)
			continue
		}
		if !strings.Contains(err.Error(), "low-order") {
			t.Errorf("low-order point #%d: error should contain 'low-order', got: %v", i, err)
		}
	}
}

func TestLowOrderPointDoesNotAffectValidKeys(t *testing.T) {
	alice, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() alice error: %v", err)
	}

	bob, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() bob error: %v", err)
	}

	shared, err := ComputeSharedSecret(alice.PrivateKey, bob.PublicKey)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() error: %v", err)
	}

	var zero [KeySize]byte
	if bytes.Equal(shared, zero[:]) {
		t.Error("shared secret between valid key pairs should not be all zeros")
	}

	if len(shared) == 0 {
		t.Error("shared secret should not be empty")
	}
}

func TestEnvelopeRoundTripAfterHKDFSaltFix(t *testing.T) {
	vaultKey, err := GenerateVaultKey()
	if err != nil {
		t.Fatalf("GenerateVaultKey() error: %v", err)
	}

	recipient, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	encKey, err := EncryptVaultKeyForUser(vaultKey, recipient.PublicKey)
	if err != nil {
		t.Fatalf("EncryptVaultKeyForUser() error: %v", err)
	}

	decrypted, err := DecryptVaultKey(encKey, recipient.PrivateKey)
	if err != nil {
		t.Fatalf("DecryptVaultKey() error: %v", err)
	}

	if !bytes.Equal(vaultKey, decrypted) {
		t.Error("decrypted vault key does not match original after HKDF salt fix")
	}
}

func TestIsLowOrderPoint(t *testing.T) {
	lowOrderPoints := [][KeySize]byte{
		// All-zeros
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// {1, 0, 0, ...}
		{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// p-1
		{0xec, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		// p
		{0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		// p+1
		{0xee, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
	}

	for i, point := range lowOrderPoints {
		if !isLowOrderPoint(point) {
			t.Errorf("low-order point #%d: isLowOrderPoint should return true", i)
		}
	}

	// Valid keys should not be flagged as low-order
	for i := 0; i < 5; i++ {
		kp, err := GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair() error: %v", err)
		}
		if isLowOrderPoint(kp.PublicKey) {
			t.Errorf("valid public key #%d was incorrectly flagged as low-order", i)
		}
	}
}
