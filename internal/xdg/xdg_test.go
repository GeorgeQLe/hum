package xdg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeDirXDGOverride(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/tmp/test-xdg-runtime")
	dir := RuntimeDir()
	if dir != "/tmp/test-xdg-runtime/humrun" {
		t.Errorf("expected /tmp/test-xdg-runtime/humrun, got %s", dir)
	}
}

func TestCacheDirXDGOverride(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/test-xdg-cache")
	dir := CacheDir()
	if dir != "/tmp/test-xdg-cache/humrun" {
		t.Errorf("expected /tmp/test-xdg-cache/humrun, got %s", dir)
	}
}

func TestDirFallbacksNotSharedTmp(t *testing.T) {
	// Clear XDG env vars to test fallbacks
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CACHE_HOME", "")

	runtimeDir := RuntimeDir()
	cacheDir := CacheDir()

	// On macOS, should use ~/Library/Caches/humrun
	// On Linux without XDG vars, should use ~/.cache/humrun
	// In all cases, should NOT be plain /tmp/humrun-sockets or /tmp/humrun-crashes
	sharedTmpPaths := []string{
		filepath.Join(os.TempDir(), "humrun-sockets"),
		filepath.Join(os.TempDir(), "humrun-crashes"),
	}

	for _, bad := range sharedTmpPaths {
		if runtimeDir == bad {
			t.Errorf("RuntimeDir() = %q, should not be shared temp path %q", runtimeDir, bad)
		}
		if cacheDir == bad {
			t.Errorf("CacheDir() = %q, should not be shared temp path %q", cacheDir, bad)
		}
	}

	// Should contain "humrun" somewhere in the path
	if !strings.Contains(runtimeDir, "humrun") {
		t.Errorf("RuntimeDir() = %q, should contain 'humrun'", runtimeDir)
	}
	if !strings.Contains(cacheDir, "humrun") {
		t.Errorf("CacheDir() = %q, should contain 'humrun'", cacheDir)
	}

	// On macOS, verify it uses ~/Library/Caches
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err == nil {
			if !strings.HasPrefix(cacheDir, filepath.Join(home, "Library", "Caches")) {
				t.Errorf("CacheDir() on macOS = %q, expected to be under ~/Library/Caches", cacheDir)
			}
		}
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "test-ensure", "nested")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat after EnsureDir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("expected 0700, got %o", info.Mode().Perm())
	}
}
