package health

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateHealthURL_Localhost(t *testing.T) {
	if err := ValidateHealthURL("http://localhost:3000/health"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateHealthURL_127001(t *testing.T) {
	if err := ValidateHealthURL("http://127.0.0.1:8080/health"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateHealthURL_IPv6Loopback(t *testing.T) {
	if err := ValidateHealthURL("http://[::1]:3000/health"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidateHealthURL_ExternalHost(t *testing.T) {
	err := ValidateHealthURL("http://evil.com:3000/health")
	if err == nil {
		t.Fatal("expected error for external host")
	}
	if !strings.Contains(err.Error(), "localhost") {
		t.Errorf("expected error containing 'localhost', got: %v", err)
	}
}

func TestValidateHealthURL_ExternalIP(t *testing.T) {
	err := ValidateHealthURL("http://10.0.0.1:3000/health")
	if err == nil {
		t.Fatal("expected error for external IP")
	}
	if !strings.Contains(err.Error(), "localhost") {
		t.Errorf("expected error containing 'localhost', got: %v", err)
	}
}

func TestValidateHealthURL_NoScheme(t *testing.T) {
	err := ValidateHealthURL("localhost:3000/health")
	if err == nil {
		t.Fatal("expected error for URL with no scheme")
	}
}

func TestValidateHealthURL_FTPScheme(t *testing.T) {
	err := ValidateHealthURL("ftp://localhost/file")
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("expected error containing 'scheme', got: %v", err)
	}
}

func TestRegisterRejectsExternalURL(t *testing.T) {
	checker := NewChecker()
	defer checker.StopAll()

	err := checker.Register("app", "http://evil.com/health", 5000)
	if err == nil {
		t.Fatal("expected error when registering external URL")
	}
}

func TestRegisterAcceptsLocalhost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	checker := NewChecker()
	defer checker.StopAll()

	err := checker.Register("app", ts.URL, 5000)
	if err != nil {
		t.Fatalf("expected no error for localhost test server, got: %v", err)
	}
}

func TestCheckerRedirectNotFollowed(t *testing.T) {
	// Target server that returns 200 — if the redirect were followed,
	// the checker would see this 200 and report healthy.
	var targetHit bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	// Redirecting server that sends a 302 redirect to the target.
	// The checker's HTTP client has CheckRedirect set to return
	// http.ErrUseLastResponse, so the redirect should NOT be followed.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	checker := NewChecker()
	defer checker.StopAll()

	err := checker.Register("app", redirector.URL, 5000)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Wait for the initial health check to complete
	time.Sleep(500 * time.Millisecond)

	// The redirect must not be followed — target should never be contacted.
	if targetHit {
		t.Error("redirect was followed; expected checker to block redirects (SSRF protection)")
	}
}
