package cmd

import (
	"strings"
	"testing"
)

func TestValidateEmail_Valid(t *testing.T) {
	valid := []string{
		"alice@example.com",
		"bob+tag@corp.co",
		"dev@localhost",
	}
	for _, email := range valid {
		if err := validateEmail(email); err != nil {
			t.Errorf("validateEmail(%q) = %v, want nil", email, err)
		}
	}
}

func TestValidateEmail_PathTraversal(t *testing.T) {
	cases := []struct {
		input   string
		wantSub string
	}{
		{"../../etc/passwd", "path separator"},
		{"../../../etc/shadow", "path separator"},
		{"foo/bar@evil.com", "path separator"},
		{"foo\\bar@evil.com", "path separator"},
		{"..@evil.com", "traversal"},
	}
	for _, tc := range cases {
		err := validateEmail(tc.input)
		if err == nil {
			t.Errorf("validateEmail(%q) = nil, want error containing %q", tc.input, tc.wantSub)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSub) {
			t.Errorf("validateEmail(%q) = %v, want error containing %q", tc.input, err, tc.wantSub)
		}
	}
}

func TestValidateEmail_InvalidFormat(t *testing.T) {
	cases := []string{
		"not-an-email",
		"",
		"@",
		"@example.com",
	}
	for _, email := range cases {
		if err := validateEmail(email); err == nil {
			t.Errorf("validateEmail(%q) = nil, want error", email)
		}
	}
}
