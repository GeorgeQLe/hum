package cmd

import (
	"fmt"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/keychain"
	"github.com/spf13/cobra"
)

func LockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Clear cached key from OS keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			v, err := vault.Open(projectRoot)
			if err != nil {
				return fmt.Errorf("no vault found: %w", err)
			}

			if err := keychain.Delete(v.Config.Project); err != nil {
				// Not an error if nothing was cached
				fmt.Println("Vault locked (no cached key found).")
				return nil
			}

			fmt.Printf("Vault locked. Cleared cached key for %q.\n", v.Config.Project)
			return nil
		},
	}
}
