package process

import (
	"net"
	"testing"
)

func TestIsPortFree(t *testing.T) {
	// Find a free port by letting the OS assign one
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	// Port should be in use
	if IsPortFree(port) {
		t.Errorf("port %d should be in use", port)
	}

	ln.Close()

	// Port should be free now
	if !IsPortFree(port) {
		t.Errorf("port %d should be free after closing listener", port)
	}
}

func TestFindFreePortAbove9999(t *testing.T) {
	port := FindFreePort(nil, 49990)
	if port == 0 {
		t.Skip("could not find free port starting at 49990")
	}
	if port < 49990 || port > 65535 {
		t.Errorf("FindFreePort(nil, 49990) = %d, want in range 49990–65535", port)
	}
}

func TestSuggestAlternativePort(t *testing.T) {
	// Use a port that's likely free
	alt := SuggestAlternativePort(49999)
	if alt == 0 {
		t.Skip("could not find alternative port (all candidates in use)")
	}
	if alt <= 49999 {
		t.Errorf("alternative port %d should be greater than base port 49999", alt)
	}
}
