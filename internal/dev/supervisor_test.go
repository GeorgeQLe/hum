package dev

import (
	"os"
	"strings"
	"testing"

	"github.com/georgele/hum/internal/process"
)

func TestSupervisorEnvFiltering(t *testing.T) {
	// Set sensitive env vars
	t.Setenv("HUMSAFE_PASSWORD", "secret123")
	t.Setenv("HUMRUN_TOKEN", "tok-abc")
	t.Setenv("HUMRUN_API_TOKEN", "api-tok-xyz")
	t.Setenv("SAFE_VAR", "visible")

	env := process.FilteredEnv()

	for _, e := range env {
		if strings.HasPrefix(e, "HUMSAFE_PASSWORD=") {
			t.Error("HUMSAFE_PASSWORD should be filtered from child env")
		}
		if strings.HasPrefix(e, "HUMRUN_TOKEN=") {
			t.Error("HUMRUN_TOKEN should be filtered from child env")
		}
		if strings.HasPrefix(e, "HUMRUN_API_TOKEN=") {
			t.Error("HUMRUN_API_TOKEN should be filtered from child env")
		}
	}

	// Verify non-sensitive vars are preserved
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "SAFE_VAR=") {
			found = true
			break
		}
	}
	if !found {
		t.Error("SAFE_VAR should be preserved in child env")
	}
}

func TestSupervisorTmpBinaryNotInSharedTmp(t *testing.T) {
	tmpDir := t.TempDir()

	s, err := New(tmpDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Temp binary should NOT be in shared /tmp directly
	if strings.HasPrefix(s.tmpBinary, os.TempDir()) && !strings.Contains(s.tmpBinary, "humrun") {
		t.Errorf("tmpBinary = %q, should not be in shared temp dir without user isolation", s.tmpBinary)
	}
}
