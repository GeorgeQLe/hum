package main

import (
	"github.com/spf13/cobra"

	"github.com/georgele/hum/internal/dev"
)

func newDevCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dev",
		Short: "Development mode with auto-rebuild on source changes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := dev.New(".")
			if err != nil {
				return err
			}
			return s.Run()
		},
	}
}
