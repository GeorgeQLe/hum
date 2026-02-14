package auth

import (
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
	uri := TOTPProvisioningURI("JBSWY3DPEHPK3PXP", "user@example.com", "envsafe")
	if uri == "" {
		t.Error("provisioning URI should not be empty")
	}
}
