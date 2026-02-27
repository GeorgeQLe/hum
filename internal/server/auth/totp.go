package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"time"
)

const (
	totpDigits = 6
	totpPeriod = 30 // seconds
)

// GenerateTOTPSecret creates a random TOTP secret.
func GenerateTOTPSecret() (string, error) {
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generating TOTP secret: %w", err)
	}
	return base32.StdEncoding.EncodeToString(secret), nil
}

// GenerateTOTPCode generates a TOTP code for the current time.
func GenerateTOTPCode(secret string, t time.Time) (string, error) {
	// Accept both padded and unpadded base32 secrets for compatibility
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		// Fall back to no-padding for legacy secrets
		key, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
		if err != nil {
			return "", fmt.Errorf("decoding secret: %w", err)
		}
	}

	counter := uint64(t.Unix()) / totpPeriod
	return generateHOTP(key, counter), nil
}

// ValidateTOTPCode checks if a TOTP code is valid (allows 1 period of drift).
func ValidateTOTPCode(secret, code string) (bool, error) {
	now := time.Now()
	for _, offset := range []time.Duration{0, -totpPeriod * time.Second, totpPeriod * time.Second} {
		expected, err := GenerateTOTPCode(secret, now.Add(offset))
		if err != nil {
			return false, err
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true, nil
		}
	}
	return false, nil
}

// TOTPProvisioningURI generates an otpauth:// URI for QR code generation.
func TOTPProvisioningURI(secret, email, issuer string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(email)
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", issuer)
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", totpPeriod))
	return fmt.Sprintf("otpauth://totp/%s?%s", label, params.Encode())
}

func generateHOTP(key []byte, counter uint64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0F
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7FFFFFFF

	otp := code % uint32(math.Pow10(totpDigits))
	return fmt.Sprintf("%06d", otp)
}
