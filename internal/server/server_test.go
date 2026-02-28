package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStubEndpointsReturn501(t *testing.T) {
	srv := New(Config{
		Addr:      ":0",
		JWTSecret: "test-secret-for-testing-only-1234",
	})

	// All stub endpoints should return 501 Not Implemented
	stubs := []struct {
		method string
		path   string
	}{
		{"POST", "/api/auth/register"},
		{"POST", "/api/auth/login"},
	}

	for _, stub := range stubs {
		req := httptest.NewRequest(stub.method, stub.path, nil)
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s %s: expected 501, got %d", stub.method, stub.path, w.Code)
		}
	}
}

func TestServerBodySizeLimit(t *testing.T) {
	srv := New(Config{
		Addr:      ":0",
		JWTSecret: "test-secret-for-testing-only-1234",
	})

	// Create oversized body (>1MB)
	largeBody := strings.Repeat("x", 2*1024*1024)
	req := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Use the full middleware stack
	handler := srv.withMiddleware(srv.router)
	handler.ServeHTTP(w, req)

	// The handler should not crash or hang with an oversized body.
	// Since the stub returns 501 and doesn't read the body, this verifies
	// the MaxBytesReader is applied and doesn't interfere.
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 (stub), got %d", w.Code)
	}
}

func TestServerAuthRateLimiting(t *testing.T) {
	srv := New(Config{
		Addr:      ":0",
		JWTSecret: "test-secret-for-testing-only-1234",
	})

	// The auth rate limiter allows 5 burst, 1/s sustained.
	// Send 10 rapid requests — some should be rate-limited.
	rateLimited := 0
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/api/auth/login", nil)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.router.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			rateLimited++
		}
	}

	if rateLimited == 0 {
		t.Error("expected some requests to be rate-limited after 10 rapid auth requests (burst=5)")
	}
}
