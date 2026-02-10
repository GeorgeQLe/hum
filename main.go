package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/ipc"
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

	rootCmd.AddCommand(pingCmd, statusCmd, addCmd)

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
		tea.WithMouseCellMotion(),
	)

	// Handle SIGTERM/SIGHUP — forward to Bubble Tea as an interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	}()

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
	if len(ports) == 0 {
		return fmt.Errorf("could not detect ports. Use --ports to specify")
	}

	entry := map[string]interface{}{
		"name":    name,
		"dir":     targetDir,
		"command": command,
		"ports":   ports,
	}

	appJSON, _ := json.Marshal(entry)
	cwd, _ := os.Getwd()

	client := ipc.NewClient(projectRoot)
	resp, err := client.AddApp(appJSON, cwd, autoStart)
	if err != nil {
		return fmt.Errorf("could not connect to devctl: %w", err)
	}

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
