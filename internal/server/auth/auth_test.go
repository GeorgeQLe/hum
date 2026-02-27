package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHashVerifyPassword(t *testing.T) {
	hash, err := HashPassword("test-password-123")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	ok, err := VerifyPassword("test-password-123", hash)
	if err != nil {
		t.Fatalf("VerifyPassword() error: %v", err)
	}
	if !ok {
		t.Error("VerifyPassword() should return true for correct password")
	}

	ok, _ = VerifyPassword("wrong-password", hash)
	if ok {
		t.Error("VerifyPassword() should return false for wrong password")
	}
}

func TestHashUniqueSalts(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Error("two hashes of the same password should be different (random salt)")
	}
}

func TestGenerateSessionToken(t *testing.T) {
	t1, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken() error: %v", err)
	}
	if len(t1) < 16 {
		t.Error("session token should be sufficiently long")
	}

	t2, _ := GenerateSessionToken()
	if t1 == t2 {
		t.Error("two session tokens should be different")
	}
}

func TestTOTPGenerate(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error: %v", err)
	}

	code, err := GenerateTOTPCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateTOTPCode() error: %v", err)
	}

	if len(code) != 6 {
		t.Errorf("TOTP code length = %d, want 6", len(code))
	}
}

func TestTOTPValidate(t *testing.T) {
	secret, _ := GenerateTOTPSecret()

	code, _ := GenerateTOTPCode(secret, time.Now())

	ok, err := ValidateTOTPCode(secret, code)
	if err != nil {
		t.Fatalf("ValidateTOTPCode() error: %v", err)
	}
	if !ok {
		t.Error("ValidateTOTPCode() should accept current code")
	}

	ok, _ = ValidateTOTPCode(secret, "000000")
	if ok {
		t.Error("ValidateTOTPCode() should reject invalid code")
	}
}

func TestTOTPProvisioningURI(t *testing.T) {
	uri := TOTPProvisioningURI("JBSWY3DPEHPK3PXP", "user@example.com", "humsafe")
	if uri == "" {
		t.Error("provisioning URI should not be empty")
	}
}

// --- JWT Security Tests ---

// testSignJWT is a helper to create a JWT with custom header/claims for testing.
func testSignJWT(t *testing.T, secret, signingInput string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestJWTGeneration(t *testing.T) {
	secret := "test-secret-key-256-bits-long!!!"
	token, err := GenerateJWT("user@example.com", secret)
	if err != nil {
		t.Fatalf("GenerateJWT() error: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}

	// Verify header contains HS256
	headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var header jwtHeader
	json.Unmarshal(headerJSON, &header)
	if header.Alg != "HS256" {
		t.Errorf("expected alg HS256, got %q", header.Alg)
	}
}

func TestJWTValidation(t *testing.T) {
	secret := "test-secret-key-256-bits-long!!!"
	token, err := GenerateJWT("user@example.com", secret)
	if err != nil {
		t.Fatalf("GenerateJWT() error: %v", err)
	}

	claims, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT() error: %v", err)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("expected email user@example.com, got %q", claims.Email)
	}
}

func TestJWTAlgorithmNone(t *testing.T) {
	// Craft a token with alg: "none" — this must be rejected
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims := JWTClaims{
		Email:     "attacker@evil.com",
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Token with empty signature (alg:none attack)
	token := header + "." + payload + "."

	_, err := ValidateJWT(token, "any-secret")
	if err == nil {
		t.Fatal("ValidateJWT() should reject alg:none tokens")
	}
	if !strings.Contains(err.Error(), "unsupported algorithm") {
		t.Errorf("expected 'unsupported algorithm' error, got: %v", err)
	}
}

func TestJWTAlgorithmRS256Rejected(t *testing.T) {
	// Craft a token claiming RS256 algorithm
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := JWTClaims{
		Email:     "attacker@evil.com",
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		IssuedAt:  time.Now().Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	token := header + "." + payload + ".fake-sig"

	_, err := ValidateJWT(token, "any-secret")
	if err == nil {
		t.Fatal("ValidateJWT() should reject RS256 tokens")
	}
}

func TestJWTExpired(t *testing.T) {
	secret := "test-secret-key-256-bits-long!!!"

	// Manually create an expired token
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := JWTClaims{
		Email:     "user@example.com",
		ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(), // expired 1 hour ago
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
	}
	claimsJSON, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := header + "." + payload
	sig := testSignJWT(t, secret, signingInput)
	token := signingInput + "." + sig

	_, err := ValidateJWT(token, secret)
	if err == nil {
		t.Fatal("ValidateJWT() should reject expired tokens")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired' error, got: %v", err)
	}
}

func TestJWTTampered(t *testing.T) {
	secret := "test-secret-key-256-bits-long!!!"
	token, _ := GenerateJWT("user@example.com", secret)

	// Tamper with the payload
	parts := strings.Split(token, ".")
	claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims JWTClaims
	json.Unmarshal(claimsJSON, &claims)
	claims.Email = "admin@evil.com"
	newClaimsJSON, _ := json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(newClaimsJSON)

	tamperedToken := strings.Join(parts, ".")

	_, err := ValidateJWT(tamperedToken, secret)
	if err == nil {
		t.Fatal("ValidateJWT() should reject tampered tokens")
	}
}

func TestJWTWrongSecret(t *testing.T) {
	token, _ := GenerateJWT("user@example.com", "secret-one")
	_, err := ValidateJWT(token, "secret-two")
	if err == nil {
		t.Fatal("ValidateJWT() should reject tokens signed with different secret")
	}
}

func TestJWTInvalidFormat(t *testing.T) {
	_, err := ValidateJWT("not.a.valid.jwt.extra", "secret")
	if err == nil {
		t.Fatal("ValidateJWT() should reject invalid format (too many parts)")
	}

	_, err = ValidateJWT("onlyone", "secret")
	if err == nil {
		t.Fatal("ValidateJWT() should reject single-part tokens")
	}

	_, err = ValidateJWT("", "secret")
	if err == nil {
		t.Fatal("ValidateJWT() should reject empty tokens")
	}
}

// --- TOTP Replay Test ---

func TestTOTPReplay(t *testing.T) {
	// Verify that validation is time-bound — codes from outside
	// the ±1 period drift window are rejected
	secret, _ := GenerateTOTPSecret()

	// Generate a code for far in the past (outside the ±1 period drift window)
	pastTime := time.Now().Add(-5 * time.Minute)
	oldCode, _ := GenerateTOTPCode(secret, pastTime)

	// This old code should not validate (outside the drift window)
	ok, _ := ValidateTOTPCode(secret, oldCode)
	if ok {
		t.Error("ValidateTOTPCode() should reject codes from >1 period ago")
	}
}
