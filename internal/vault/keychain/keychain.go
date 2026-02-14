package keychain

import (
	"github.com/zalando/go-keyring"
)

const (
	serviceName = "envsafe"
)

// Store caches the derived encryption key in the OS keychain.
// The key is stored per project (using projectName as the account).
func Store(projectName string, password string) error {
	return keyring.Set(serviceName, projectName, password)
}

// Retrieve gets the cached password from the OS keychain.
func Retrieve(projectName string) (string, error) {
	return keyring.Get(serviceName, projectName)
}

// Delete removes the cached password from the OS keychain.
func Delete(projectName string) error {
	return keyring.Delete(serviceName, projectName)
}
