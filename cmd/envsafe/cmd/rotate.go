package cmd

import (
	"fmt"

	"github.com/georgele/devctl/internal/vault"
	"github.com/spf13/cobra"
)

func RotateCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "rotate <key> <new-value>",
		Short: "Rotate a secret (stores previous value in history)",
		Args:  cobra.ExactArgs(2),
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
