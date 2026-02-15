package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalDir returns the path to ~/.devctl/.
func GlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".devctl")
	}
	return filepath.Join(home, ".devctl")
}

// DiscoveryInfo is written to ~/.devctl/api.json for client discovery.
type DiscoveryInfo struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// discoveryPath returns the path to ~/.devctl/api.json.
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

// WriteDiscovery writes the discovery file to ~/.devctl/api.json (chmod 600).
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

// ReadDiscovery reads the discovery file from ~/.devctl/api.json.
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
