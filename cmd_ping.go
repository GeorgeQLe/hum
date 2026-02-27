package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/georgele/hum/internal/api"
	"github.com/georgele/hum/internal/ipc"
)

func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Check if humrun is running",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPing()
		},
	}
}

func runPing() error {
	// Try HTTP API first
	if apiClient, err := api.NewClientFromDiscovery(); err == nil {
		if err := apiClient.Health(); err == nil {
			info, _ := api.ReadDiscovery()
			fmt.Printf("humrun is running (PID %d) — API on port %d\n", info.PID, info.Port)
			return nil
		}
	}

	// Fallback to IPC
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	client := ipc.NewClient(projectRoot)
	resp, err := client.Ping()
	if err != nil {
		return fmt.Errorf("humrun is not running: %w", err)
	}

	if resp.OK {
		fmt.Printf("humrun is running (PID %d) for project: %s\n", resp.PID, resp.Project)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}
	return nil
}
