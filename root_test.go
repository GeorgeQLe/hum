package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/georgele/hum/internal/xdg"
)

func TestCrashLogLocation(t *testing.T) {
	crashDir := filepath.Join(xdg.CacheDir(), "crashes")

	// Crash dir should NOT be in shared /tmp
	sharedTmp := filepath.Join(os.TempDir(), "humrun-crashes")
	if crashDir == sharedTmp {
		t.Errorf("crash dir = %q, should not be shared /tmp path", crashDir)
	}

	// Should be under user's home or XDG cache
	home, err := os.UserHomeDir()
	if err == nil {
		if !strings.HasPrefix(crashDir, home) {
			t.Errorf("crash dir = %q, should be under user home %q", crashDir, home)
		}
	}

	// Should contain "humrun" in the path
	if !strings.Contains(crashDir, "humrun") {
		t.Errorf("crash dir = %q, should contain 'humrun'", crashDir)
	}
}
