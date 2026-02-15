package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/dev"
	"github.com/georgele/devctl/internal/ipc"
	"github.com/georgele/devctl/internal/process"
	"github.com/georgele/devctl/internal/tui"
)

func main() {
	var startAll bool
	var restore bool

	rootCmd := &cobra.Command{
		Use:   "devctl",
		Short: "Multi-app dev server manager",
		Long:  "devctl — A TUI for managing multiple local development servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(startAll, restore)
		},
	}

	rootCmd.Flags().BoolVar(&startAll, "start-all", false, "Start all apps on launch")
	rootCmd.Flags().BoolVar(&restore, "restore", false, "Restore previous session")

	// IPC client subcommands
	pingCmd := &cobra.Command{
		Use:   "ping",
		Short: "Check if devctl is running",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPing()
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show running instance status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus()
		},
	}

	var addName string
	var addCommand string
	var addPorts string
	var addStart bool

	addCmd := &cobra.Command{
		Use:   "add [dir]",
		Short: "Add app from directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return runAdd(dir, addName, addCommand, addPorts, addStart)
		},
	}

	addCmd.Flags().StringVar(&addName, "name", "", "Override detected app name")
	addCmd.Flags().StringVar(&addCommand, "command", "", "Override detected command")
	addCmd.Flags().StringVar(&addPorts, "ports", "", "Override detected ports (comma-separated)")
	addCmd.Flags().BoolVar(&addStart, "start", false, "Start the app after adding")

	var statsWatch bool
	var statsJSON bool

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show resource statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(statsWatch, statsJSON)
		},
	}

	statsCmd.Flags().BoolVar(&statsWatch, "watch", false, "Continuously update every 2s")
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Output as JSON")

	var scanJSON bool
	var scanWrite bool

	scanCmd := &cobra.Command{
		Use:   "scan",
		Short: "Auto-detect apps in project tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(scanJSON, scanWrite)
		},
	}

	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "Output as JSON")
	scanCmd.Flags().BoolVar(&scanWrite, "write", false, "Write detected apps to apps.json")

	devCmd := &cobra.Command{
		Use:   "dev",
		Short: "Development mode with auto-rebuild on source changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dev.New(".").Run()
		},
	}

	startCmd := &cobra.Command{
		Use:   "start <name|all>",
		Short: "Start an app (auto-resolves port conflicts)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppAction("start", args[0])
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop <name|all>",
		Short: "Stop an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppAction("stop", args[0])
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart <name|all>",
		Short: "Restart an app",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppAction("restart", args[0])
		},
	}

	rootCmd.AddCommand(pingCmd, statusCmd, addCmd, statsCmd, scanCmd, devCmd, startCmd, stopCmd, restartCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUI(startAll, restore bool) error {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	apps, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	model := tui.New(projectRoot, apps, startAll, restore)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	// Handle SIGTERM/SIGHUP — forward to Bubble Tea as an interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	// Handle SIGUSR1 — dev reload (graceful restart without killing managed apps)
	devReloadCh := make(chan os.Signal, 1)
	signal.Notify(devReloadCh, syscall.SIGUSR1)
	defer signal.Stop(devReloadCh)

	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			defer func() { recover() }()
			p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		case <-devReloadCh:
			defer func() { recover() }()
			p.Send(tui.DevReloadMsg{})
		case <-done:
			return
		}
	}()
	defer close(done)

	// Panic recovery to restore terminal state
	defer func() {
		if r := recover(); r != nil {
			p.Kill()
			fmt.Fprintf(os.Stderr, "devctl panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}

func runPing() error {
	// Try HTTP API first
	if apiClient, err := api.NewClientFromDiscovery(); err == nil {
		if err := apiClient.Health(); err == nil {
			info, _ := api.ReadDiscovery()
			fmt.Printf("devctl is running (PID %d) — API on port %d\n", info.PID, info.Port)
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
		return fmt.Errorf("devctl is not running: %w", err)
	}

	if resp.OK {
		fmt.Printf("devctl is running (PID %d) for project: %s\n", resp.PID, resp.Project)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
		os.Exit(1)
	}
	return nil
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

func runAdd(dir, nameFlag, commandFlag, portsFlag string, autoStart bool) error {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	targetDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	// Try to auto-detect from package.json
	scan, scanErr := config.ScanCurrentDir(targetDir, projectRoot)

	var name, command string
	var ports []int
	var env map[string]string

	if scan != nil {
		name = scan.Name
		command = scan.Command
		ports = scan.Ports
	}

	// Apply flag overrides
	if nameFlag != "" {
		name = nameFlag
	}
	if commandFlag != "" {
		command = commandFlag
	}
	if portsFlag != "" {
		ports = nil
		for _, s := range strings.Split(portsFlag, ",") {
			s = strings.TrimSpace(s)
			var p int
			if _, err := fmt.Sscanf(s, "%d", &p); err == nil && p > 0 && p < 65536 {
				ports = append(ports, p)
			}
		}
	}

	if name == "" {
		if scanErr != nil {
			return fmt.Errorf("could not detect app: %v", scanErr)
		}
		return fmt.Errorf("could not detect app name. Use --name to specify")
	}

	// Auto-assign port when none detected
	if len(ports) == 0 {
		apps, loadErr := config.Load(projectRoot)
		if loadErr != nil {
			return fmt.Errorf("could not load config: %w", loadErr)
		}
		var usedPorts []int
		for _, a := range apps {
			usedPorts = append(usedPorts, a.Ports...)
		}
		port := process.FindFreePort(usedPorts, 3000)
		if port == 0 {
			return fmt.Errorf("could not find a free port. Use --ports to specify")
		}
		ports = []int{port}
		env = map[string]string{"PORT": fmt.Sprintf("%d", port)}
		fmt.Printf("Auto-assigned port %d for \"%s\" (set via PORT env var)\n", port, name)
	}

	entry := map[string]interface{}{
		"name":    name,
		"dir":     targetDir,
		"command": command,
		"ports":   ports,
	}

	appJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling app entry: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Try IPC first (TUI running)
	client := ipc.NewClient(projectRoot)
	resp, err := client.AddApp(appJSON, cwd, autoStart)
	if err == nil {
		// IPC succeeded
		if resp.OK {
			fmt.Printf("Added \"%s\" to devctl.\n", resp.Name)
			if autoStart {
				fmt.Printf("App \"%s\" is starting.\n", resp.Name)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			os.Exit(1)
		}
		return nil
	}

	// Offline fallback: write directly to apps.json
	apps, loadErr := config.Load(projectRoot)
	if loadErr != nil {
		return fmt.Errorf("could not load config: %w", loadErr)
	}

	// Make dir relative to project root
	relDir, relErr := filepath.Rel(projectRoot, targetDir)
	if relErr != nil || strings.HasPrefix(relDir, "..") {
		return fmt.Errorf("directory is outside the project root")
	}
	if relDir == "" {
		relDir = "."
	}

	// Check for duplicates
	for _, a := range apps {
		if a.Name == name {
			return fmt.Errorf("app %q already exists", name)
		}
		if a.Dir == relDir {
			return fmt.Errorf("directory %q is already registered as %q", relDir, a.Name)
		}
	}

	app := config.App{
		Name:    name,
		Dir:     relDir,
		Command: command,
		Ports:   ports,
		Env:     env,
	}
	if err := app.Validate(); err != nil {
		return fmt.Errorf("invalid app: %w", err)
	}

	apps = append(apps, app)
	if err := config.Save(projectRoot, apps); err != nil {
		return fmt.Errorf("could not save config: %w", err)
	}

	fmt.Printf("Added \"%s\" to apps.json (offline mode).\n", name)
	if autoStart {
		fmt.Fprintf(os.Stderr, "Warning: --start requires the devctl TUI to be running.\n")
	}

	return nil
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

func runScan(jsonOut, autoWrite bool) error {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	apps, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	candidates := config.DetectApps(projectRoot, apps)

	if len(candidates) == 0 {
		fmt.Println("No new apps detected.")
		return nil
	}

	if jsonOut {
		data, err := json.MarshalIndent(candidates, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if !autoWrite {
		fmt.Printf("Detected %d app(s):\n\n", len(candidates))
		fmt.Printf("  %-20s %-30s %-20s %s\n", "NAME", "DIR", "COMMAND", "PORTS")
		for _, c := range candidates {
			portStr := "auto"
			if len(c.Ports) > 0 {
				portStrs := make([]string, len(c.Ports))
				for i, p := range c.Ports {
					portStrs[i] = fmt.Sprintf("%d", p)
				}
				portStr = strings.Join(portStrs, ",")
			}
			fmt.Printf("  %-20s %-30s %-20s %s\n", c.Name, c.Dir, c.Command, portStr)
		}
		fmt.Printf("\nRun with --write to add these to apps.json.\n")
		return nil
	}

	// --write mode: add candidates to apps.json
	existingNames := make(map[string]bool)
	for _, a := range apps {
		existingNames[a.Name] = true
	}

	// Collect used ports for auto-assignment
	var usedPorts []int
	for _, a := range apps {
		usedPorts = append(usedPorts, a.Ports...)
	}

	var added []string
	for _, c := range candidates {
		name := c.Name

		// Auto-resolve name collisions
		if existingNames[name] {
			suffix := 2
			for existingNames[fmt.Sprintf("%s-%d", c.Name, suffix)] {
				suffix++
			}
			name = fmt.Sprintf("%s-%d", c.Name, suffix)
		}

		ports := c.Ports
		var env map[string]string
		if len(ports) == 0 {
			port := process.FindFreePort(usedPorts, 3000)
			if port == 0 {
				fmt.Fprintf(os.Stderr, "warning: skipping %q: no free port available\n", name)
				continue
			}
			ports = []int{port}
			env = map[string]string{"PORT": fmt.Sprintf("%d", port)}
			usedPorts = append(usedPorts, port)
			fmt.Printf("  Auto-assigned port %d for \"%s\" (set via PORT env var)\n", port, name)
		}

		app := config.App{
			Name:    name,
			Dir:     c.Dir,
			Command: c.Command,
			Ports:   ports,
			Env:     env,
		}
		if err := app.Validate(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: %v\n", name, err)
			continue
		}

		apps = append(apps, app)
		existingNames[name] = true
		added = append(added, name)
	}

	if len(added) == 0 {
		fmt.Println("No valid apps to add.")
		return nil
	}

	if err := config.Save(projectRoot, apps); err != nil {
		return fmt.Errorf("could not save config: %w", err)
	}

	fmt.Printf("Added %d app(s) to apps.json:\n", len(added))
	for _, name := range added {
		fmt.Printf("  + %s\n", name)
	}
	return nil
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.0fK", float64(b)/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1fM", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.1fG", float64(b)/(1024*1024*1024))
}

// findProjectRoot walks up from CWD to find a directory with apps.json.
// Falls back to CWD if not found.
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		configPath := filepath.Join(dir, "apps.json")
		if _, err := os.Stat(configPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// No apps.json found, use CWD
	return cwd, nil
}
