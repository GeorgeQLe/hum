package panicutil

import (
	"bytes"
	"os"
	"testing"
)

func TestRecoverCatchesPanic(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover("test")
		panic("test panic")
	}()
	<-done
	// If we reach here, the goroutine completed without crashing the process.
}

func TestRecoverNoopWithoutPanic(t *testing.T) {
	result := func() string {
		defer Recover("test")
		return "ok"
	}()
	if result != "ok" {
		t.Fatalf("expected 'ok', got %q", result)
	}
}

func TestRecoverWritesToStderr(t *testing.T) {
	// Save original stderr and replace with a pipe.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover("test")
		panic("kaboom")
	}()
	<-done

	// Close the write end and read captured output.
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()
	os.Stderr = origStderr

	output := buf.String()
	if len(output) == 0 {
		t.Fatal("expected stderr output, got nothing")
	}
	if !bytes.Contains([]byte(output), []byte("panic in test")) {
		t.Fatalf("expected stderr to contain 'panic in test', got: %s", output)
	}
}
