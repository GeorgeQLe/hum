package cmd

import (
	"fmt"

	"github.com/georgele/devctl/internal/vault"
	"github.com/spf13/cobra"
)

func RmCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "rm <key>",
		Short: "Remove a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			v, err := openAndUnlock(projectRoot)
			if err != nil {
				return err
			}
			defer v.Lock()

			if err := v.Remove(env, key); err != nil {
				return err
			}

			fmt.Printf("Removed %s from %q environment.\n", key, env)
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")

	return cmd
}
