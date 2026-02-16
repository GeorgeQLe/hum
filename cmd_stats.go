package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/ipc"
)

func newStatsCmd() *cobra.Command {
	var statsWatch bool
	var statsJSON bool

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show resource statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(statsWatch, statsJSON)
		},
	}

	cmd.Flags().BoolVar(&statsWatch, "watch", false, "Continuously update every 2s")
	cmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")

	return cmd
}

func runStats(watch, jsonOut bool) error {
	for {
		// Try HTTP API first
		if apiClient, apiErr := api.NewClientFromDiscovery(); apiErr == nil {
			data, err := apiClient.Stats()
			if err == nil {
				if jsonOut {
					fmt.Println(string(data))
				} else {
					if watch {
						fmt.Print("\033[2J\033[H")
					}
					var resp struct {
						Apps []struct {
							Name    string  `json:"name"`
							Status  string  `json:"status"`
							PID     int     `json:"pid"`
							CPU     float64 `json:"cpu"`
							MemRSS  int64   `json:"memRss"`
							MaxCPU  float64 `json:"maxCpu"`
							MaxMem  int64   `json:"maxMem"`
							Uptime  string  `json:"uptime"`
							Samples int     `json:"samples"`
						} `json:"apps"`
					}
					if json.Unmarshal(data, &resp) == nil {
						info, _ := api.ReadDiscovery()
						if info != nil {
							fmt.Printf("devctl (PID %d) — API on port %d\n\n", info.PID, info.Port)
						}
						fmt.Printf("  %-20s %-10s %8s %10s %10s %10s %s\n",
							"NAME", "STATUS", "CPU", "MEM", "PEAK CPU", "PEAK MEM", "UPTIME")
						for _, app := range resp.Apps {
							cpuStr, memStr, peakCPU, peakMem, uptime := "-", "-", "-", "-", "-"
							if app.Status == "running" {
								cpuStr = fmt.Sprintf("%.1f%%", app.CPU)
								memStr = formatBytes(app.MemRSS)
								if app.Samples > 0 {
									peakCPU = fmt.Sprintf("%.1f%%", app.MaxCPU)
									peakMem = formatBytes(app.MaxMem)
								}
								uptime = app.Uptime
							}
							icon := "\033[90m○\033[0m"
							if app.Status == "running" {
								icon = "\033[32m●\033[0m"
							}
							fmt.Printf("  %s %-20s %-10s %8s %10s %10s %10s %s\n",
								icon, app.Name, app.Status, cpuStr, memStr, peakCPU, peakMem, uptime)
						}
					}
				}
				if !watch {
					return nil
				}
				time.Sleep(2 * time.Second)
				continue
			}
		}

		// Fallback to IPC
		projectRoot, err := findProjectRoot()
		if err != nil {
			return fmt.Errorf("could not find project root: %w", err)
		}

		client := ipc.NewClient(projectRoot)
		resp, err := client.Stats()
		if err != nil {
			return fmt.Errorf("devctl is not running: %w", err)
		}

		if !resp.OK {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}

		if jsonOut {
			if resp.Apps != nil {
				fmt.Println(string(resp.Apps))
			} else {
				fmt.Println("[]")
			}
		} else {
			if watch {
				fmt.Print("\033[2J\033[H")
			}
			fmt.Printf("devctl (PID %d) — %s\n\n", resp.PID, resp.Project)

			if resp.Apps == nil {
				fmt.Println("  No apps configured.")
			} else {
				type appStats struct {
					Name    string  `json:"name"`
					Status  string  `json:"status"`
					PID     int     `json:"pid"`
					CPU     float64 `json:"cpu"`
					MemRSS  int64   `json:"memRss"`
					AvgCPU  float64 `json:"avgCpu"`
					MaxCPU  float64 `json:"maxCpu"`
					AvgMem  int64   `json:"avgMem"`
					MaxMem  int64   `json:"maxMem"`
					Uptime  string  `json:"uptime"`
					Samples int     `json:"samples"`
				}

				var apps []appStats
				if err := json.Unmarshal(resp.Apps, &apps); err != nil {
					return fmt.Errorf("invalid response: %w", err)
				}

				fmt.Printf("  %-20s %-10s %8s %10s %10s %10s %s\n",
					"NAME", "STATUS", "CPU", "MEM", "PEAK CPU", "PEAK MEM", "UPTIME")

				for _, app := range apps {
					cpuStr := "-"
					memStr := "-"
					peakCPU := "-"
					peakMem := "-"
					uptime := "-"

					if app.Status == "running" {
						cpuStr = fmt.Sprintf("%.1f%%", app.CPU)
						memStr = formatBytes(app.MemRSS)
						if app.Samples > 0 {
							peakCPU = fmt.Sprintf("%.1f%%", app.MaxCPU)
							peakMem = formatBytes(app.MaxMem)
						}
						uptime = app.Uptime
					}

					icon := "\033[90m○\033[0m"
					if app.Status == "running" {
						icon = "\033[32m●\033[0m"
					}

					fmt.Printf("  %s %-20s %-10s %8s %10s %10s %10s %s\n",
						icon, app.Name, app.Status, cpuStr, memStr, peakCPU, peakMem, uptime)
				}
			}
		}

		if !watch {
			break
		}
		time.Sleep(2 * time.Second)
	}

	return nil
}
