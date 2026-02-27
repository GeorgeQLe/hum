package cmd

import (
	"fmt"

	"github.com/georgele/hum/internal/vault"
	"github.com/spf13/cobra"
)

func SetCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a secret",
		Long: `Store an encrypted secret in the vault for the specified environment.
The key must match [A-Za-z_][A-Za-z0-9_.-]*.`,
		Example: `  humsafe set API_KEY sk-1234
  humsafe set -e production DATABASE_URL postgres://...`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			if err := validateName(env, "environment"); err != nil {
				return err
			}
			if err := validateName(key, "key"); err != nil {
				return err
			}

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'humsafe init' first")
			}

			v, err := openAndUnlock(projectRoot)
			if err != nil {
				return err
			}
			defer v.Lock()

			if err := v.Set(env, key, value); err != nil {
				return err
			}

			fmt.Printf("Set %s in %q environment.\n", key, env)
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")

	return cmd
}
