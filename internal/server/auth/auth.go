package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// JWTClaims represents the payload of a JWT.
type JWTClaims struct {
	Email     string `json:"email"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
}

// GenerateJWT creates an HMAC-SHA256 signed JWT for the given email.
func GenerateJWT(email, secret string) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

	claims := JWTClaims{
		Email:     email,
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshalling claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

// ValidateJWT verifies an HMAC-SHA256 JWT and returns claims.
func ValidateJWT(tokenStr, secret string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decoding claims: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("unmarshalling claims: %w", err)
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

// User represents an authenticated user.
type User struct {
	ID        string
	Email     string
	Role      string // admin, developer, viewer
	TenantID  string
	TOTPEnabled bool
}

// HashPassword creates an Argon2id hash of the password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)

	// Format: $argon2id$salt$hash (both base64)
	return fmt.Sprintf("$argon2id$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword checks a password against an Argon2id hash.
// Hash format: $argon2id$<base64-salt>$<base64-hash>
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// Expected: ["", "argon2id", "<salt>", "<hash>"]
	if len(parts) != 4 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, fmt.Errorf("decoding salt: %w", err)
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, fmt.Errorf("decoding hash: %w", err)
	}

	computedHash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)

	return subtle.ConstantTimeCompare(expectedHash, computedHash) == 1, nil
}

// Session represents an active user session.
type Session struct {
	Token     string
	UserID    string
	TenantID  string
	ExpiresAt time.Time
}

// GenerateSessionToken creates a cryptographically random session token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
