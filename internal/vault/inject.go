package vault

import (
	"fmt"
	"os"
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

		// Try to unlock using cached password from env var (set by unlock command)
		password := os.Getenv("ENVSAFE_PASSWORD")
		if password == "" {
			// Try keychain
			return nil, fmt.Errorf("vault is locked. Run 'envsafe unlock' first")
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
