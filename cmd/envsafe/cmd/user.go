package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/sharing"
)

func UserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage team members",
	}

	addCmd := &cobra.Command{
		Use:   "add <email>",
		Short: "Invite a team member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'envsafe init' first")
			}

			// Load team config
			tc, err := loadTeamConfig(projectRoot)
			if err != nil {
				tc = &sharing.TeamConfig{}
			}

			// Check if user already exists
			for _, m := range tc.Members {
				if m.Email == email {
					return fmt.Errorf("user %q is already a team member", email)
				}
			}

			// Generate a key pair for the new user
			kp, err := sharing.GenerateKeyPair()
			if err != nil {
				return fmt.Errorf("generating key pair: %w", err)
			}

			tc.Members = append(tc.Members, sharing.TeamMember{
				Email:     email,
				PublicKey: kp.PublicKeyBase64(),
				Role:      "developer",
				AddedAt:   time.Now().UTC().Format(time.RFC3339),
			})

			if err := saveTeamConfig(projectRoot, tc); err != nil {
				return err
			}

			fmt.Printf("Added %s as a team member (role: developer).\n", email)
			fmt.Printf("Private key (share securely with user):\n  %s\n", kp.PrivateKeyBase64())
			fmt.Printf("Public key stored in vault config.\n")

			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List team members",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			tc, err := loadTeamConfig(projectRoot)
			if err != nil {
				fmt.Println("No team members configured.")
				return nil
			}

			if len(tc.Members) == 0 {
				fmt.Println("No team members configured.")
				return nil
			}

			fmt.Println("Team members:")
			for _, m := range tc.Members {
				fmt.Printf("  %s (role: %s, added: %s)\n", m.Email, m.Role, m.AddedAt)
			}
			return nil
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove <email>",
		Short: "Remove a team member",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			tc, err := loadTeamConfig(projectRoot)
			if err != nil {
				return fmt.Errorf("no team configuration found")
			}

			found := false
			members := make([]sharing.TeamMember, 0, len(tc.Members))
			for _, m := range tc.Members {
				if m.Email == email {
					found = true
					continue
				}
				members = append(members, m)
			}

			if !found {
				return fmt.Errorf("user %q not found", email)
			}

			tc.Members = members

			// Remove their encrypted keys
			keys := make([]sharing.EncryptedVaultKey, 0, len(tc.Keys))
			for _, k := range tc.Keys {
				if k.Email != email {
					keys = append(keys, k)
				}
			}
			tc.Keys = keys

			if err := saveTeamConfig(projectRoot, tc); err != nil {
				return err
			}

			fmt.Printf("Removed %s from team.\n", email)
			return nil
		},
	}

	roleCmd := &cobra.Command{
		Use:   "role <email> <role>",
		Short: "Set user role (admin, developer, viewer)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			email, role := args[0], args[1]

			if role != "admin" && role != "developer" && role != "viewer" {
				return fmt.Errorf("invalid role %q (must be admin, developer, or viewer)", role)
			}

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			tc, err := loadTeamConfig(projectRoot)
			if err != nil {
				return fmt.Errorf("no team configuration found")
			}

			found := false
			for i, m := range tc.Members {
				if m.Email == email {
					tc.Members[i].Role = role
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("user %q not found", email)
			}

			if err := saveTeamConfig(projectRoot, tc); err != nil {
				return err
			}

			fmt.Printf("Set %s role to %s.\n", email, role)
			return nil
		},
	}

	cmd.AddCommand(addCmd, listCmd, removeCmd, roleCmd)
	return cmd
}

func teamConfigPath(projectRoot string) string {
	return filepath.Join(vault.VaultPath(projectRoot), "team.json")
}

func loadTeamConfig(projectRoot string) (*sharing.TeamConfig, error) {
	data, err := os.ReadFile(teamConfigPath(projectRoot))
	if err != nil {
		return nil, err
	}
	return sharing.UnmarshalTeamConfig(data)
}

func saveTeamConfig(projectRoot string, tc *sharing.TeamConfig) error {
	data, err := sharing.MarshalTeamConfig(tc)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(teamConfigPath(projectRoot), data, 0644)
}
