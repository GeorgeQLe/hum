package sharing

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/georgele/hum/internal/vault/crypto"
	"golang.org/x/crypto/hkdf"
)

// EncryptedVaultKey is a vault key encrypted for a specific team member.
type EncryptedVaultKey struct {
	Email        string `json:"email"`
	PublicKey    string `json:"public_key"`     // Recipient's public key (base64)
	EncryptedKey string `json:"encrypted_key"`  // Vault key encrypted with shared secret (base64)
	EphemeralPub string `json:"ephemeral_pub"`  // Ephemeral public key for this encryption (base64)
}

// TeamConfig stores team sharing configuration in the vault config.
type TeamConfig struct {
	Members []TeamMember      `json:"members"`
	Keys    []EncryptedVaultKey `json:"keys"`
}

// TeamMember represents a team member's identity.
type TeamMember struct {
	Email     string `json:"email"`
	PublicKey string `json:"public_key"` // base64-encoded X25519 public key
	Role      string `json:"role"`       // admin, developer, viewer
	AddedAt   string `json:"added_at"`
}

// EncryptVaultKeyForUser encrypts a vault key for a specific user using
// X25519 key agreement + AES-256-GCM (envelope encryption).
//
// Process:
// 1. Generate ephemeral X25519 key pair
// 2. Compute shared secret: X25519(ephemeral_private, recipient_public)
// 3. Derive AES key from shared secret using SHA-256
// 4. Encrypt vault key with AES-256-GCM using derived key
func EncryptVaultKeyForUser(vaultKey []byte, recipientPubKey [KeySize]byte) (*EncryptedVaultKey, error) {
	// Generate ephemeral key pair
	ephemeral, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generating ephemeral key: %w", err)
	}
	defer func() { for i := range ephemeral.PrivateKey { ephemeral.PrivateKey[i] = 0 } }()

	// Compute shared secret
	shared, err := ComputeSharedSecret(ephemeral.PrivateKey, recipientPubKey)
	if err != nil {
		return nil, fmt.Errorf("computing shared secret: %w", err)
	}
	defer func() { for i := range shared { shared[i] = 0 } }()

	// Derive AES key from shared secret using HKDF
	hkdfReader := hkdf.New(sha256.New, shared, nil, []byte("humsafe-vault-key-sharing-v1"))
	var aesKey [32]byte
	if _, err := io.ReadFull(hkdfReader, aesKey[:]); err != nil {
		return nil, fmt.Errorf("deriving AES key: %w", err)
	}
	defer func() { for i := range aesKey { aesKey[i] = 0 } }()

	// Encrypt vault key
	encrypted, err := crypto.Encrypt(aesKey[:], vaultKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting vault key: %w", err)
	}

	return &EncryptedVaultKey{
		PublicKey:    base64.StdEncoding.EncodeToString(recipientPubKey[:]),
		EncryptedKey: base64.StdEncoding.EncodeToString(encrypted),
		EphemeralPub: ephemeral.PublicKeyBase64(),
	}, nil
}

// DecryptVaultKey decrypts a vault key using the recipient's private key.
//
// Process:
// 1. Decode ephemeral public key
// 2. Compute shared secret: X25519(recipient_private, ephemeral_public)
// 3. Derive AES key from shared secret using SHA-256
// 4. Decrypt vault key with AES-256-GCM
func DecryptVaultKey(encKey *EncryptedVaultKey, recipientPrivKey [KeySize]byte) ([]byte, error) {
	// Decode ephemeral public key
	ephPub, err := PublicKeyFromBase64(encKey.EphemeralPub)
	if err != nil {
		return nil, fmt.Errorf("decoding ephemeral key: %w", err)
	}

	// Compute shared secret
	shared, err := ComputeSharedSecret(recipientPrivKey, ephPub)
	if err != nil {
		return nil, fmt.Errorf("computing shared secret: %w", err)
	}
	defer func() { for i := range shared { shared[i] = 0 } }()

	// Derive AES key using HKDF
	hkdfReader := hkdf.New(sha256.New, shared, nil, []byte("humsafe-vault-key-sharing-v1"))
	var aesKey [32]byte
	if _, err := io.ReadFull(hkdfReader, aesKey[:]); err != nil {
		return nil, fmt.Errorf("deriving AES key: %w", err)
	}
	defer func() { for i := range aesKey { aesKey[i] = 0 } }()

	// Decode encrypted vault key
	ciphertext, err := base64.StdEncoding.DecodeString(encKey.EncryptedKey)
	if err != nil {
		return nil, fmt.Errorf("decoding encrypted key: %w", err)
	}

	// Decrypt
	vaultKey, err := crypto.Decrypt(aesKey[:], ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypting vault key: %w", err)
	}

	return vaultKey, nil
}

// GenerateVaultKey generates a random 256-bit key for vault encryption.
func GenerateVaultKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating vault key: %w", err)
	}
	return key, nil
}

// MarshalTeamConfig serializes team configuration to JSON.
func MarshalTeamConfig(tc *TeamConfig) ([]byte, error) {
	return json.MarshalIndent(tc, "", "  ")
}

// UnmarshalTeamConfig deserializes team configuration from JSON.
func UnmarshalTeamConfig(data []byte) (*TeamConfig, error) {
	var tc TeamConfig
	if err := json.Unmarshal(data, &tc); err != nil {
		return nil, fmt.Errorf("parsing team config: %w", err)
	}
	return &tc, nil
}
