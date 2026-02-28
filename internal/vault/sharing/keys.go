package sharing

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

const KeySize = 32

// KeyPair holds an X25519 key pair.
type KeyPair struct {
	PrivateKey [KeySize]byte
	PublicKey  [KeySize]byte
}

// GenerateKeyPair generates a new X25519 key pair.
func GenerateKeyPair() (*KeyPair, error) {
	var kp KeyPair

	if _, err := rand.Read(kp.PrivateKey[:]); err != nil {
		return nil, fmt.Errorf("generating private key: %w", err)
	}

	pub, err := curve25519.X25519(kp.PrivateKey[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("computing public key: %w", err)
	}
	copy(kp.PublicKey[:], pub)

	return &kp, nil
}

// PrivateKeyBase64 returns the base64-encoded private key.
func (kp *KeyPair) PrivateKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PrivateKey[:])
}

// PublicKeyBase64 returns the base64-encoded public key.
func (kp *KeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.PublicKey[:])
}

// PublicKeyFromBase64 decodes a base64-encoded public key.
func PublicKeyFromBase64(s string) ([KeySize]byte, error) {
	var key [KeySize]byte
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return key, fmt.Errorf("decoding public key: %w", err)
	}
	if len(b) != KeySize {
		return key, fmt.Errorf("invalid public key length: %d", len(b))
	}
	copy(key[:], b)
	return key, nil
}

// PrivateKeyFromBase64 decodes a base64-encoded private key.
func PrivateKeyFromBase64(s string) ([KeySize]byte, error) {
	var key [KeySize]byte
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return key, fmt.Errorf("decoding private key: %w", err)
	}
	if len(b) != KeySize {
		return key, fmt.Errorf("invalid private key length: %d", len(b))
	}
	copy(key[:], b)
	return key, nil
}

// isLowOrderPoint returns true if the given point is a known low-order point
// on Curve25519. These points produce an all-zero shared secret and must be
// rejected to prevent key compromise.
func isLowOrderPoint(point [KeySize]byte) bool {
	// Known low-order points on Curve25519 that produce all-zero output.
	knownLowOrder := [][KeySize]byte{
		// 0 (neutral element)
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// 1
		{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		// p-1 (2^255 - 20)
		{0xec, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		// p (2^255 - 19)
		{0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
		// p+1
		{0xee, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f},
	}
	for _, lop := range knownLowOrder {
		if bytes.Equal(point[:], lop[:]) {
			return true
		}
	}
	return false
}

// ComputeSharedSecret computes an X25519 shared secret from a private key
// and a peer's public key. Returns an error if the peer's public key is a
// known low-order point.
func ComputeSharedSecret(privateKey, peerPublicKey [KeySize]byte) ([]byte, error) {
	if isLowOrderPoint(peerPublicKey) {
		return nil, fmt.Errorf("rejecting low-order public key")
	}
	shared, err := curve25519.X25519(privateKey[:], peerPublicKey[:])
	if err != nil {
		return nil, fmt.Errorf("computing shared secret: %w", err)
	}
	return shared, nil
}
