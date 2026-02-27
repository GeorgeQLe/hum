package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/ipc"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show running instance status",
		Long:  "Show the status of all managed apps in the current devctl instance.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}
}

func runStatus() error {
	// Try HTTP API first
	if apiClient, err := api.NewClientFromDiscovery(); err == nil {
		data, err := apiClient.Status()
		if err == nil {
			var resp struct {
				Apps []struct {
					Name   string `json:"name"`
					Status string `json:"status"`
					PID    int    `json:"pid"`
					Ports  []int  `json:"ports"`
				} `json:"apps"`
			}
			if json.Unmarshal(data, &resp) == nil {
				info, _ := api.ReadDiscovery()
				fmt.Printf("devctl (PID %d) — API on port %d\n", info.PID, info.Port)
				if len(resp.Apps) == 0 {
					fmt.Println("  No apps configured.")
					return nil
				}
				for _, app := range resp.Apps {
					icon := "\033[90m○\033[0m"
					if app.Status == "running" {
						icon = "\033[32m●\033[0m"
					} else if app.Status == "stopping" {
						icon = "\033[33m●\033[0m"
					}
					pidStr := ""
					if app.PID > 0 {
						pidStr = fmt.Sprintf(" (PID %d)", app.PID)
					}
					portStrs := make([]string, len(app.Ports))
					for i, p := range app.Ports {
						portStrs[i] = fmt.Sprintf("%d", p)
					}
					portStr := ""
					if len(portStrs) > 0 {
						portStr = " :" + strings.Join(portStrs, ",")
					}
					fmt.Printf("  %s %s%s%s — %s\n", icon, app.Name, pidStr, portStr, app.Status)
				}
				return nil
			}
		}
	}

	// Fallback to IPC
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	client := ipc.NewClient(projectRoot)
	resp, err := client.Status()
	if err != nil {
		return fmt.Errorf("devctl is not running: %w", err)
	}

	if !resp.OK {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}

	fmt.Printf("devctl (PID %d) — %s\n", resp.PID, resp.Project)

	if resp.Apps == nil {
		fmt.Println("  No apps configured.")
		return nil
	}

	type appStatus struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		PID    int    `json:"pid"`
		Ports  []int  `json:"ports"`
	}

	var apps []appStatus
	if err := json.Unmarshal(resp.Apps, &apps); err != nil {
		return fmt.Errorf("invalid response: %w", err)
	}

	for _, app := range apps {
		icon := "\033[90m○\033[0m" // dim
		if app.Status == "running" {
			icon = "\033[32m●\033[0m" // green
		} else if app.Status == "stopping" {
			icon = "\033[33m●\033[0m" // yellow
		}
		pidStr := ""
		if app.PID > 0 {
			pidStr = fmt.Sprintf(" (PID %d)", app.PID)
		}
		portStrs := make([]string, len(app.Ports))
		for i, p := range app.Ports {
			portStrs[i] = fmt.Sprintf("%d", p)
		}
		portStr := ""
		if len(portStrs) > 0 {
			portStr = " :" + strings.Join(portStrs, ",")
		}
		fmt.Printf("  %s %s%s%s — %s\n", icon, app.Name, pidStr, portStr, app.Status)
	}

	return nil
}
