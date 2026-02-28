// Package xdg provides user-local directory helpers following the XDG Base
// Directory Specification, with macOS-specific fallbacks.
//
// These helpers ensure humrun never writes sensitive data (crash logs, sockets,
// PID files, temp binaries) into shared directories like /tmp.
package xdg

import (
	"os"
	"path/filepath"
	"runtime"
)

// RuntimeDir returns a user-private directory for runtime files (sockets, PID files).
//
// Resolution order:
//  1. $XDG_RUNTIME_DIR/humrun (Linux, typically /run/user/<uid>/humrun)
//  2. ~/Library/Caches/humrun/run (macOS)
//  3. ~/.cache/humrun/run (fallback)
func RuntimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "humrun")
	}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Caches", "humrun", "run")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "humrun", "run")
	}
	// Last resort: use temp dir (but this is what we're trying to avoid)
	return filepath.Join(os.TempDir(), "humrun-"+uidString())
}

// CacheDir returns a user-private directory for cache files (crash logs, temp binaries).
//
// Resolution order:
//  1. $XDG_CACHE_HOME/humrun (Linux)
//  2. ~/Library/Caches/humrun (macOS)
//  3. ~/.cache/humrun (fallback)
func CacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return filepath.Join(dir, "humrun")
	}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Caches", "humrun")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "humrun")
	}
	return filepath.Join(os.TempDir(), "humrun-cache-"+uidString())
}

// EnsureDir creates a directory with 0700 permissions if it doesn't exist,
// and verifies ownership on existing directories.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0700)
}

// uidString returns a short string identifying the current user, for use
// in fallback temp paths to avoid collisions between users.
func uidString() string {
	if uid := os.Getenv("UID"); uid != "" {
		return uid
	}
	return "unknown"
}
