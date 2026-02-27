package keychain

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "humsafe"
)

// accountKey returns a collision-resistant keychain account identifier.
// Uses the absolute project path hash to prevent collisions between
// projects with the same name in different directories.
func accountKey(projectPath string) string {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		// Fall back to raw project path if Abs fails
		abs = projectPath
	}
	hash := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(hash[:8]) + ":" + filepath.Base(abs)
}

// Store caches the derived encryption key in the OS keychain.
// The key is stored per project using a hash of the absolute path
// to prevent collisions between projects with the same name.
func Store(projectPath string, password string) error {
	return keyring.Set(serviceName, accountKey(projectPath), password)
}

// Retrieve gets the cached password from the OS keychain.
func Retrieve(projectPath string) (string, error) {
	// Try new format first
	pw, err := keyring.Get(serviceName, accountKey(projectPath))
	if err == nil {
		return pw, nil
	}
	// Fall back to legacy format (just project name) for migration
	return keyring.Get(serviceName, filepath.Base(projectPath))
}

// Delete removes the cached password from the OS keychain.
func Delete(projectPath string) error {
	// Delete both new and legacy entries
	_ = keyring.Delete(serviceName, filepath.Base(projectPath)) // ignore legacy delete error
	return keyring.Delete(serviceName, accountKey(projectPath))
}
