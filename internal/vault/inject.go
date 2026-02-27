package vault

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/georgele/hum/internal/vault/keychain"
)

// ResolveEnv returns the final environment map for an app.
// If vaultEnv is non-empty, it merges vault secrets with the plain-text env.
// Plain-text env values take precedence over vault values (explicit overrides).
func ResolveEnv(projectRoot string, plainEnv map[string]string, vaultEnv string) (map[string]string, error) {
	result := make(map[string]string)

	// Load vault secrets if vault_env is specified
	if vaultEnv != "" && Exists(projectRoot) {
		v, err := Open(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("opening vault: %w", err)
		}

		// Try to unlock using cached password from env var or keychain
		password := os.Getenv("HUMSAFE_PASSWORD")
		if password != "" {
			fmt.Fprintln(os.Stderr, "warning: using HUMSAFE_PASSWORD env var — visible in process listings. Prefer keychain caching ('humsafe unlock').")
		}
		if password == "" {
			password, _ = keychain.Retrieve(filepath.Base(projectRoot))
		}
		if password == "" {
			return nil, fmt.Errorf("vault is locked. Run 'humsafe unlock' or set HUMSAFE_PASSWORD")
		}

		if err := v.Unlock(password); err != nil {
			return nil, fmt.Errorf("unlocking vault: %w", err)
		}
		defer v.Lock()

		secrets, err := v.Env(vaultEnv)
		if err != nil {
			return nil, fmt.Errorf("reading vault environment %q: %w", vaultEnv, err)
		}

		for k, val := range secrets {
			result[k] = val
		}
	}

	// Plain-text env overrides vault values
	for k, v := range plainEnv {
		result[k] = v
	}

	return result, nil
}

// ResolveEnvWithPassword returns the final environment map, using the given password.
func ResolveEnvWithPassword(projectRoot string, plainEnv map[string]string, vaultEnv string, password string) (map[string]string, error) {
	result := make(map[string]string)

	if vaultEnv != "" && Exists(projectRoot) {
		v, err := Open(projectRoot)
		if err != nil {
			return nil, fmt.Errorf("opening vault: %w", err)
		}

		if err := v.Unlock(password); err != nil {
			return nil, fmt.Errorf("unlocking vault: %w", err)
		}
		defer v.Lock()

		secrets, err := v.Env(vaultEnv)
		if err != nil {
			return nil, fmt.Errorf("reading vault environment %q: %w", vaultEnv, err)
		}

		for k, val := range secrets {
			result[k] = val
		}
	}

	for k, v := range plainEnv {
		result[k] = v
	}

	return result, nil
}
