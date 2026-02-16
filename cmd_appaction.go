package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/ipc"
)

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <name|all>",
		Short: "Start an app (auto-resolves port conflicts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppAction("start", args[0])
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name|all>",
		Short: "Stop an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppAction("stop", args[0])
		},
	}
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart <name|all>",
		Short: "Restart an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppAction("restart", args[0])
		},
	}
}

func runAppAction(action, target string) error {
	// Try HTTP API first
	if apiClient, err := api.NewClientFromDiscovery(); err == nil {
		var data []byte
		var apiErr error
		switch action {
		case "start":
			data, apiErr = apiClient.StartApp(target)
		case "stop":
			data, apiErr = apiClient.StopApp(target)
		case "restart":
			data, apiErr = apiClient.RestartApp(target)
		default:
			return fmt.Errorf("unknown action: %s", action)
		}
		if apiErr != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", apiErr)
			os.Exit(1)
		}
		var resp struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &resp) == nil && resp.Message != "" {
			fmt.Println(resp.Message)
		} else {
			fmt.Printf("%s %s\n", target, action)
		}
		return nil
	}

	// Fallback to IPC
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	client := ipc.NewClient(projectRoot)
	var resp *ipc.Response

	switch action {
	case "start":
		resp, err = client.StartApp(target)
	case "stop":
		resp, err = client.StopApp(target)
	case "restart":
		resp, err = client.RestartApp(target)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	if err != nil {
		return fmt.Errorf("devctl is not running: %w", err)
	}

	if resp.OK {
		fmt.Println(resp.Message)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}
	return nil
}
