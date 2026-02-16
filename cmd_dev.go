package main

import (
	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/dev"
)

func newDevCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dev",
		Short: "Development mode with auto-rebuild on source changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dev.New(".").Run()
		},
	}
}
