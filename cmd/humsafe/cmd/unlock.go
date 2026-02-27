package cmd

import (
	"fmt"
	"os"

	"github.com/georgele/hum/internal/vault"
	"github.com/georgele/hum/internal/vault/keychain"
	"github.com/spf13/cobra"
)

func UnlockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlock",
		Short: "Unlock vault and cache key in OS keychain",
		Long:  "Unlock the vault and cache the password in the OS keychain for subsequent commands.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			v, err := vault.Open(projectRoot)
			if err != nil {
				return fmt.Errorf("no vault found: %w", err)
			}

			password, err := promptPassword("Enter vault password: ")
			if err != nil {
				return err
			}

			if err := v.Unlock(password); err != nil {
				return err
			}
			v.Lock() // Don't keep unlocked in memory after caching

			// Store in keychain
			if err := keychain.Store(v.Config.Project, password); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not cache in keychain: %v\n", err)
				return nil
			}

			fmt.Printf("Vault unlocked. Password cached in OS keychain for %q.\n", v.Config.Project)
			return nil
		},
	}
}
