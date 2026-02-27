package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/georgele/hum/internal/config"
	"github.com/georgele/hum/internal/process"
)

func newScanCmd() *cobra.Command {
	var scanJSON bool
	var scanWrite bool

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Auto-detect apps in project tree",
		Long: `Auto-detect apps in the project tree by looking for common frameworks and tools.
Detected apps can be written to apps.json with --write.`,
		Example: `  humrun scan
  humrun scan --write
  humrun scan --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(scanJSON, scanWrite)
		},
	}

	cmd.Flags().BoolVar(&scanJSON, "json", false, "Output as JSON")
	cmd.Flags().BoolVar(&scanWrite, "write", false, "Write detected apps to apps.json")

	return cmd
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
