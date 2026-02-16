package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/ipc"
	"github.com/georgele/devctl/internal/process"
)

func newAddCmd() *cobra.Command {
	var addName string
	var addCommand string
	var addPorts string
	var addStart bool

	cmd := &cobra.Command{
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

	cmd.Flags().StringVar(&addName, "name", "", "Override detected app name")
	cmd.Flags().StringVar(&addCommand, "command", "", "Override detected command")
	cmd.Flags().StringVar(&addPorts, "ports", "", "Override detected ports (comma-separated)")
	cmd.Flags().BoolVar(&addStart, "start", false, "Start the app after adding")

	return cmd
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
