package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

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

			if err := validateName(env, "environment"); err != nil {
				return err
			}
			if err := validateName(key, "key"); err != nil {
				return err
			}

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				fmt.Fprintf(os.Stderr, "Remove %s/%s? [y/N] ", env, key)
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(answer)) != "y" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

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

			if err := v.Remove(env, key); err != nil {
				return err
			}

			fmt.Printf("Removed %s from %q environment.\n", key, env)
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Target environment")
	cmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	return cmd
}
