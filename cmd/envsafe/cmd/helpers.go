package cmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/keychain"
	"golang.org/x/term"
)

var validNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

func validateName(name, label string) error {
	if !validNameRe.MatchString(name) {
		return fmt.Errorf("invalid %s name %q: must match [A-Za-z_][A-Za-z0-9_.-]*", label, name)
	}
	return nil
}

func warnIfInsecureHTTP(serverURL string) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" && !strings.HasPrefix(serverURL, "https://") {
		fmt.Fprintln(os.Stderr, "WARNING: Using unencrypted HTTP with a non-local server. Credentials may be exposed.")
	}
}

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
		fmt.Fprintln(os.Stderr, "warning: cached keychain password is stale, prompting for password")
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
