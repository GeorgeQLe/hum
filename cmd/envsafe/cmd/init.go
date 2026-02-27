package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/keychain"
)

func InitCmd() *cobra.Command {
	var projectName string
	var importEnv bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new encrypted vault",
		Long: "Creates a .envsafe/ directory with an encrypted vault for storing secrets.",
		Example: `  envsafe init --name myproject
  envsafe init --import`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if vault.Exists(projectRoot) {
				return fmt.Errorf("vault already exists in %s", projectRoot)
			}

			if projectName == "" {
				projectName = filepath.Base(projectRoot)
			}

			password, err := promptPasswordConfirm("Create vault password: ")
			if err != nil {
				return err
			}

			v, err := vault.Init(projectRoot, projectName, password)
			if err != nil {
				return err
			}

			// Cache password in keychain
			if err := keychain.Store(projectName, password); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not cache password in keychain: %v\n", err)
			}

			fmt.Printf("Vault initialized in %s/.envsafe/\n", projectRoot)
			fmt.Printf("Project: %s\n", projectName)

			// Auto-import from apps.json if requested
			if importEnv {
				if err := importFromAppsJSON(projectRoot, v); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: import failed: %v\n", err)
				}
			} else {
				// Check if apps.json has env vars and suggest import
				if hasPlaintextEnv(projectRoot) {
					fmt.Println("\nDetected plain-text env vars in apps.json.")
					fmt.Println("Run 'envsafe init --import' to import them into the vault.")
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&projectName, "name", "", "Project name (defaults to directory name)")
	cmd.Flags().BoolVar(&importEnv, "import", false, "Import env vars from apps.json")

	return cmd
}

// hasPlaintextEnv checks if apps.json has any plain-text env vars.
func hasPlaintextEnv(projectRoot string) bool {
	data, err := os.ReadFile(filepath.Join(projectRoot, "apps.json"))
	if err != nil {
		return false
	}

	var apps []struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &apps); err != nil {
		return false
	}

	for _, app := range apps {
		if len(app.Env) > 0 {
			return true
		}
	}
	return false
}

// importFromAppsJSON imports env vars from apps.json into the vault.
func importFromAppsJSON(projectRoot string, v *vault.Vault) error {
	data, err := os.ReadFile(filepath.Join(projectRoot, "apps.json"))
	if err != nil {
		return fmt.Errorf("reading apps.json: %w", err)
	}

	var apps []struct {
		Name string            `json:"name"`
		Env  map[string]string `json:"env"`
	}
	if err := json.Unmarshal(data, &apps); err != nil {
		return fmt.Errorf("parsing apps.json: %w", err)
	}

	imported := 0
	for _, app := range apps {
		if len(app.Env) == 0 {
			continue
		}
		for k, val := range app.Env {
			if err := v.Set(vault.DefaultEnv, k, val); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not import %s: %v\n", k, err)
				continue
			}
			imported++
		}
	}

	if imported > 0 {
		fmt.Printf("Imported %d secrets from apps.json into %q environment.\n", imported, vault.DefaultEnv)
		fmt.Println("You can now remove plain-text env vars from apps.json and use vault_env instead.")
	}

	return nil
}
