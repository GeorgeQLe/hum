package cmd

import (
	"fmt"
	"os"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/audit"
	"github.com/georgele/devctl/internal/vault/keychain"
	"github.com/spf13/cobra"
)

func PasswdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "passwd",
		Short: "Change the vault master password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'envsafe init' first")
			}

			// Unlock with current password
			v, err := openAndUnlock(projectRoot)
			if err != nil {
				return err
			}
			defer v.Lock()

			// Prompt for new password
			newPassword, err := promptPasswordConfirm("New vault password: ")
			if err != nil {
				return err
			}

			if err := v.ChangePassword(newPassword); err != nil {
				return fmt.Errorf("changing password: %w", err)
			}

			// Update keychain cache
			if err := keychain.Store(v.Config.Project, newPassword); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not update keychain: %v\n", err)
			}

			// Audit log
			logger := audit.NewLogger(vault.VaultPath(projectRoot))
			if err := logger.Log(audit.Entry{
				Action:  "password_change",
				User:    "local",
				Details: "vault master password changed",
			}); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to write audit log: %v\n", err)
			}

			fmt.Println("Vault password changed successfully.")
			return nil
		},
	}
}
