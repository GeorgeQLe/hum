package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/keychain"
	"golang.org/x/term"
)

// findProjectRoot walks up from CWD to find a directory with .envsafe/ or apps.json.
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		// Check for .envsafe directory
		if _, err := os.Stat(filepath.Join(dir, vault.VaultDir)); err == nil {
			return dir, nil
		}
		// Check for apps.json
		if _, err := os.Stat(filepath.Join(dir, "apps.json")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return cwd, nil
}

// openAndUnlock opens and unlocks a vault, using keychain if available.
func openAndUnlock(projectRoot string) (*vault.Vault, error) {
	v, err := vault.Open(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("no vault found: %w", err)
	}

	// Try keychain first
	password, err := keychain.Retrieve(v.Config.Project)
	if err == nil && password != "" {
		if err := v.Unlock(password); err == nil {
			return v, nil
		}
		// Cached password is stale; fall through to prompt
	}

	// Prompt for password
	password, err = promptPassword("Enter vault password: ")
	if err != nil {
		return nil, err
	}

	if err := v.Unlock(password); err != nil {
		return nil, err
	}

	return v, nil
}

// promptPassword reads a password from the terminal without echoing.
func promptPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after password
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return string(b), nil
}

// promptPasswordConfirm prompts for a password twice and verifies they match.
func promptPasswordConfirm(prompt string) (string, error) {
	pass1, err := promptPassword(prompt)
	if err != nil {
		return "", err
	}
	if pass1 == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	pass2, err := promptPassword("Confirm password: ")
	if err != nil {
		return "", err
	}

	if pass1 != pass2 {
		return "", fmt.Errorf("passwords do not match")
	}

	return pass1, nil
}
