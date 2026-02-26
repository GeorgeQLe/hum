package cmd

import (
	"fmt"

	"github.com/georgele/devctl/internal/vault"
	"github.com/spf13/cobra"
)

func ListCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secret keys (not values)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'envsafe init' first")
			}

			v, err := openAndUnlock(projectRoot)
			if err != nil {
				return err
			}
			defer v.Lock()

			keys, err := v.List(env)
			if err != nil {
				return err
			}

			if len(keys) == 0 {
				fmt.Printf("No secrets in %q environment.\n", env)
				return nil
			}

			fmt.Printf("Secrets in %q environment:\n", env)
			for _, k := range keys {
				fmt.Printf("  %s\n", k)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")

	return cmd
}
