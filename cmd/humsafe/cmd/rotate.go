package cmd

import (
	"fmt"

	"github.com/georgele/hum/internal/vault"
	"github.com/spf13/cobra"
)

func RotateCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "rotate <key> <new-value>",
		Short: "Rotate a secret (stores previous value in history)",
		Long: `Update a secret's value and store the previous value in rotation history.
Use this instead of 'set' when you want to keep a record of previous values.`,
		Example: `  humsafe rotate API_KEY sk-new-value
  humsafe rotate -e production DB_PASSWORD newpass123`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, newValue := args[0], args[1]

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

			if err := v.Rotate(env, key, newValue); err != nil {
				return err
			}

			fmt.Printf("Rotated %s in %q environment. Previous value saved to history.\n", key, env)
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")

	return cmd
}
