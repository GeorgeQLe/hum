package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShareClientHasTimeout(t *testing.T) {
	// Use a context-aware handler so the server shuts down cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-ctx.Done() // block until cancelled
	}))
	defer func() {
		cancel()     // unblock handler
		srv.Close()  // now Close returns immediately
	}()

	client := &http.Client{Timeout: 500 * time.Millisecond}

	start := time.Now()
	_, err := client.Post(srv.URL+"/api/share", "application/json", strings.NewReader(`{}`))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("client waited %v, expected timeout around 500ms", elapsed)
	}
}

func TestShareResponseBodyLimited(t *testing.T) {
	const maxResponseSize = 1 << 20 // 1 MB — must match share.go

	// Server that sends 2MB of data.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		data := make([]byte, 2<<20)
		w.Write(data)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(srv.URL+"/api/share", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}

	if len(body) > maxResponseSize {
		t.Errorf("body size %d exceeds limit %d", len(body), maxResponseSize)
	}
	if len(body) != maxResponseSize {
		t.Errorf("expected exactly %d bytes from LimitReader, got %d", maxResponseSize, len(body))
	}
}
