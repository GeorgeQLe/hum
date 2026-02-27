package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/georgele/hum/cmd/humsafe/cmd"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "humsafe",
		Short: "Encrypted environment variable manager",
		Long:  "humsafe — Encrypted secrets manager with humrun integration",
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
		cmd.PasswdCmd(),
		cmd.BackupCmd(),
		cmd.RestoreCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
