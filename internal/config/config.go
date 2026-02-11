package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// App represents a single application entry in apps.json.
type App struct {
	Name         string `json:"name"`
	Dir          string `json:"dir"`
	Command      string `json:"command"`
	Ports        []int  `json:"ports"`
	AutoRestart  *bool  `json:"autoRestart,omitempty"`
	RestartDelay *int   `json:"restartDelay,omitempty"`
	MaxRestarts  *int   `json:"maxRestarts,omitempty"`
}

// Validate checks that an App entry has all required fields.
func (a *App) Validate() error {
	if a.Name == "" {
		return fmt.Errorf("missing or invalid \"name\"")
	}
	if a.Dir == "" {
		return fmt.Errorf("missing or invalid \"dir\"")
	}
	if a.Command == "" {
		return fmt.Errorf("missing or invalid \"command\"")
	}
	if len(a.Ports) == 0 {
		return fmt.Errorf("\"ports\" must be a non-empty array of integers 1-65535")
	}
	for _, p := range a.Ports {
		if p < 1 || p > 65535 {
			return fmt.Errorf("\"ports\" must be a non-empty array of integers 1-65535")
		}
	}
	if a.RestartDelay != nil && *a.RestartDelay < 0 {
		return fmt.Errorf("\"restartDelay\" must be a non-negative number")
	}
	if a.MaxRestarts != nil && *a.MaxRestarts < 0 {
		return fmt.Errorf("\"maxRestarts\" must be a non-negative integer")
	}
	return nil
}

// ConfigPath returns the path to apps.json for the given project root.
func ConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, "apps.json")
}

// Load reads and parses apps.json from the project root.
// Creates an empty apps.json if the file doesn't exist.
// Returns an error if the file contains invalid JSON.
func Load(projectRoot string) ([]App, error) {
	configPath := ConfigPath(projectRoot)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Auto-create empty config file (B9)
			if writeErr := os.WriteFile(configPath, []byte("[]\n"), 0644); writeErr != nil {
				return nil, fmt.Errorf("could not create %s: %w", configPath, writeErr)
			}
			return []App{}, nil
		}
		return nil, err
	}

	var apps []App
	if err := json.Unmarshal(data, &apps); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", configPath, err)
	}

	valid := make([]App, 0, len(apps))
	for _, app := range apps {
		if app.Validate() == nil {
			valid = append(valid, app)
		}
	}
	return valid, nil
}

// Save writes the apps list to apps.json.
func Save(projectRoot string, apps []App) error {
	configPath := ConfigPath(projectRoot)

	// Clean the entries for serialization
	clean := make([]App, len(apps))
	for i, a := range apps {
		clean[i] = App{
			Name:    a.Name,
			Dir:     a.Dir,
			Command: a.Command,
			Ports:   a.Ports,
		}
		if a.AutoRestart != nil {
			clean[i].AutoRestart = a.AutoRestart
		}
		if a.RestartDelay != nil {
			clean[i].RestartDelay = a.RestartDelay
		}
		if a.MaxRestarts != nil {
			clean[i].MaxRestarts = a.MaxRestarts
		}
	}

	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(configPath, data, 0644)
}

// HasChanged checks if two App entries differ in dir, command, or ports.
func HasChanged(old, new App) bool {
	if old.Dir != new.Dir || old.Command != new.Command {
		return true
	}
	if len(old.Ports) != len(new.Ports) {
		return true
	}
	for i := range old.Ports {
		if old.Ports[i] != new.Ports[i] {
			return true
		}
	}
	return false
}
