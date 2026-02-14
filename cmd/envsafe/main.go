package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/cmd/envsafe/cmd"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "envsafe",
		Short: "Encrypted environment variable manager",
		Long:  "envsafe — Encrypted secrets manager with devctl integration",
	}

	rootCmd.AddCommand(
		cmd.InitCmd(),
		cmd.SetCmd(),
		cmd.GetCmd(),
		cmd.ListCmd(),
		cmd.RmCmd(),
		cmd.EnvCmd(),
		cmd.UnlockCmd(),
		cmd.LockCmd(),
		cmd.RotateCmd(),
		cmd.BrowseCmd(),
		cmd.UserCmd(),
		cmd.AuditCmd(),
		cmd.ShareCmd(),
		cmd.ServeCmd(),
		cmd.LoginCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
