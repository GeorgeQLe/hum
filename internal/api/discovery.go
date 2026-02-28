package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalDir returns the path to ~/.humrun/.
// Falls back to a user-scoped XDG runtime dir if home dir is unavailable,
// avoiding shared /tmp to prevent symlink attacks.
func GlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Avoid shared /tmp — use XDG runtime dir or a UID-scoped fallback
		if xdgRuntime := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntime != "" {
			return filepath.Join(xdgRuntime, "humrun")
		}
		return filepath.Join(os.TempDir(), fmt.Sprintf("humrun-%d", os.Getuid()))
	}
	return filepath.Join(home, ".humrun")
}

// DiscoveryInfo is written to ~/.humrun/api.json for client discovery.
type DiscoveryInfo struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// discoveryPath returns the path to ~/.humrun/api.json.
func discoveryPath() string {
	return filepath.Join(GlobalDir(), "api.json")
}

// GenerateToken creates a cryptographically random bearer token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// WriteDiscovery writes the discovery file to ~/.humrun/api.json (chmod 600).
func WriteDiscovery(info DiscoveryInfo) error {
	dir := GlobalDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := discoveryPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadDiscovery reads the discovery file from ~/.humrun/api.json.
func ReadDiscovery() (*DiscoveryInfo, error) {
	data, err := os.ReadFile(discoveryPath())
	if err != nil {
		return nil, err
	}
	var info DiscoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("invalid discovery file: %w", err)
	}
	return &info, nil
}

// RemoveDiscovery deletes the discovery file.
func RemoveDiscovery() {
	os.Remove(discoveryPath())
}
