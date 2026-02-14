package sharing

import (
	"bytes"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}

	// Keys should not be all zeros
	var zero [KeySize]byte
	if kp.PrivateKey == zero {
		t.Error("private key should not be all zeros")
	}
	if kp.PublicKey == zero {
		t.Error("public key should not be all zeros")
	}

	// Two key pairs should be different
	kp2, _ := GenerateKeyPair()
	if kp.PrivateKey == kp2.PrivateKey {
		t.Error("two generated key pairs should have different private keys")
	}
}

func TestKeyBase64RoundTrip(t *testing.T) {
	kp, _ := GenerateKeyPair()

	pubB64 := kp.PublicKeyBase64()
	decoded, err := PublicKeyFromBase64(pubB64)
	if err != nil {
		t.Fatalf("PublicKeyFromBase64() error: %v", err)
	}
	if decoded != kp.PublicKey {
		t.Error("public key round-trip failed")
	}

	privB64 := kp.PrivateKeyBase64()
	decodedPriv, err := PrivateKeyFromBase64(privB64)
	if err != nil {
		t.Fatalf("PrivateKeyFromBase64() error: %v", err)
	}
	if decodedPriv != kp.PrivateKey {
		t.Error("private key round-trip failed")
	}
}

func TestComputeSharedSecret(t *testing.T) {
	alice, _ := GenerateKeyPair()
	bob, _ := GenerateKeyPair()

	// Alice computes shared secret with Bob's public key
	secret1, err := ComputeSharedSecret(alice.PrivateKey, bob.PublicKey)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() error: %v", err)
	}

	// Bob computes shared secret with Alice's public key
	secret2, err := ComputeSharedSecret(bob.PrivateKey, alice.PublicKey)
	if err != nil {
		t.Fatalf("ComputeSharedSecret() error: %v", err)
	}

	// Both should be the same (Diffie-Hellman)
	if !bytes.Equal(secret1, secret2) {
		t.Error("shared secrets should match (Diffie-Hellman)")
	}
}

func TestEnvelopeEncryptDecrypt(t *testing.T) {
	// Generate a vault key
	vaultKey, err := GenerateVaultKey()
	if err != nil {
		t.Fatalf("GenerateVaultKey() error: %v", err)
	}

	// Generate recipient key pair
	recipient, _ := GenerateKeyPair()

	// Encrypt vault key for recipient
	encKey, err := EncryptVaultKeyForUser(vaultKey, recipient.PublicKey)
	if err != nil {
		t.Fatalf("EncryptVaultKeyForUser() error: %v", err)
	}

	// Decrypt with recipient's private key
	decrypted, err := DecryptVaultKey(encKey, recipient.PrivateKey)
	if err != nil {
		t.Fatalf("DecryptVaultKey() error: %v", err)
	}

	if !bytes.Equal(vaultKey, decrypted) {
		t.Error("decrypted vault key should match original")
	}
}

func TestEnvelopeDecryptWrongKey(t *testing.T) {
	vaultKey, _ := GenerateVaultKey()
	recipient, _ := GenerateKeyPair()
	wrongKey, _ := GenerateKeyPair()

	encKey, _ := EncryptVaultKeyForUser(vaultKey, recipient.PublicKey)

	// Try to decrypt with wrong private key
	_, err := DecryptVaultKey(encKey, wrongKey.PrivateKey)
	if err == nil {
		t.Error("DecryptVaultKey() with wrong key should error")
	}
}

func TestMultipleRecipients(t *testing.T) {
	vaultKey, _ := GenerateVaultKey()

	// Create 3 team members
	alice, _ := GenerateKeyPair()
	bob, _ := GenerateKeyPair()
	charlie, _ := GenerateKeyPair()

	// Encrypt for each
	encAlice, _ := EncryptVaultKeyForUser(vaultKey, alice.PublicKey)
	encBob, _ := EncryptVaultKeyForUser(vaultKey, bob.PublicKey)
	encCharlie, _ := EncryptVaultKeyForUser(vaultKey, charlie.PublicKey)

	// Each can decrypt
	for _, tc := range []struct {
		name    string
		enc     *EncryptedVaultKey
		privKey [KeySize]byte
	}{
		{"alice", encAlice, alice.PrivateKey},
		{"bob", encBob, bob.PrivateKey},
		{"charlie", encCharlie, charlie.PrivateKey},
	} {
		decrypted, err := DecryptVaultKey(tc.enc, tc.privKey)
		if err != nil {
			t.Errorf("%s: DecryptVaultKey() error: %v", tc.name, err)
			continue
		}
		if !bytes.Equal(vaultKey, decrypted) {
			t.Errorf("%s: decrypted key doesn't match", tc.name)
		}
	}
}

func TestTeamConfigSerialization(t *testing.T) {
	tc := &TeamConfig{
		Members: []TeamMember{
			{Email: "alice@example.com", PublicKey: "base64key", Role: "admin", AddedAt: "2024-01-01T00:00:00Z"},
		},
		Keys: []EncryptedVaultKey{
			{Email: "alice@example.com", PublicKey: "pubkey", EncryptedKey: "enckey", EphemeralPub: "ephkey"},
		},
	}

	data, err := MarshalTeamConfig(tc)
	if err != nil {
		t.Fatalf("MarshalTeamConfig() error: %v", err)
	}

	tc2, err := UnmarshalTeamConfig(data)
	if err != nil {
		t.Fatalf("UnmarshalTeamConfig() error: %v", err)
	}

	if len(tc2.Members) != 1 {
		t.Errorf("members count = %d, want 1", len(tc2.Members))
	}
	if tc2.Members[0].Email != "alice@example.com" {
		t.Errorf("member email = %q, want %q", tc2.Members[0].Email, "alice@example.com")
	}
}
