//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/georgele/hum/internal/health"
)

func TestHealthCheckTransition(t *testing.T) {
	// Start a real HTTP server that always returns 200.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	checker := health.NewChecker()
	defer checker.StopAll()

	// Register with 1-second interval (minimum allowed).
	// The initial check runs immediately in the background goroutine.
	if err := checker.Register("healthy-app", srv.URL, 1000); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Wait for the status change from unknown to healthy.
	select {
	case change := <-checker.Changes():
		if change.AppName != "healthy-app" {
			t.Errorf("app name: got %q, want healthy-app", change.AppName)
		}
		if change.OldStatus != health.StatusUnknown {
			t.Errorf("old status: got %q, want unknown", change.OldStatus)
		}
		if change.NewStatus != health.StatusHealthy {
			t.Errorf("new status: got %q, want healthy", change.NewStatus)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for healthy status change")
	}

	if got := checker.GetStatus("healthy-app"); got != health.StatusHealthy {
		t.Errorf("GetStatus: got %q, want healthy", got)
	}
}

func TestHealthCheckUnhealthy(t *testing.T) {
	// Get a port with nothing listening.
	port := FreeTCPPort(t)
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)

	checker := health.NewChecker()
	defer checker.StopAll()

	if err := checker.Register("dead-app", url, 1000); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// The initial check should fail immediately (connection refused).
	select {
	case change := <-checker.Changes():
		if change.AppName != "dead-app" {
			t.Errorf("app name: got %q, want dead-app", change.AppName)
		}
		if change.NewStatus != health.StatusUnhealthy {
			t.Errorf("new status: got %q, want unhealthy", change.NewStatus)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for unhealthy status change")
	}

	if got := checker.GetStatus("dead-app"); got != health.StatusUnhealthy {
		t.Errorf("GetStatus: got %q, want unhealthy", got)
	}
}
