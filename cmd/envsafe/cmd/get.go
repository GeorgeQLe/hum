package cmd

import (
	"fmt"

	"github.com/georgele/devctl/internal/vault"
	"github.com/spf13/cobra"
)

func GetCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Retrieve a secret",
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

			value, err := v.Get(env, key)
			if err != nil {
				return err
			}

			fmt.Println(value)
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")

	return cmd
}
