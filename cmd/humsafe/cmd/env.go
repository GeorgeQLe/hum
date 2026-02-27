package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/georgele/hum/internal/vault"
	"github.com/spf13/cobra"
)

func EnvCmd() *cobra.Command {
	var env string

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Export all secrets as KEY=VALUE",
		Long: `Export all secrets as KEY='VALUE' pairs, suitable for shell eval or .env files.
Output is sorted alphabetically for deterministic results.`,
		Example: `  humsafe env
  humsafe env -e production > .env
  eval $(humsafe env)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			secrets, err := v.Env(env)
			if err != nil {
				return err
			}

			// Sort keys for deterministic output
			keys := make([]string, 0, len(secrets))
			for k := range secrets {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				fmt.Printf("%s='%s'\n", k, strings.ReplaceAll(secrets[k], "'", "'\\''"))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")

	return cmd
}
