package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// HealthCheckConfig defines optional HTTP health checking for an app.
type HealthCheckConfig struct {
	URL      string `json:"url"`
	Interval int    `json:"interval"` // milliseconds
}

// ResourceLimitsConfig defines optional resource thresholds for an app.
type ResourceLimitsConfig struct {
	MaxCPU      float64 `json:"maxCpu,omitempty"`
	MaxMemoryMB int64   `json:"maxMemoryMB,omitempty"`
}

// App represents a single application entry in apps.json.
type App struct {
	Name           string                `json:"name"`
	Dir            string                `json:"dir"`
	Command        string                `json:"command"`
	Ports          []int                 `json:"ports"`
	AutoRestart    *bool                 `json:"autoRestart,omitempty"`
	RestartDelay   *int                  `json:"restartDelay,omitempty"`
	MaxRestarts    *int                  `json:"maxRestarts,omitempty"`
	Env            map[string]string     `json:"env,omitempty"`
	DependsOn      []string              `json:"dependsOn,omitempty"`
	Group          string                `json:"group,omitempty"`
	HealthCheck    *HealthCheckConfig    `json:"healthCheck,omitempty"`
	Pinned         *bool                 `json:"pinned,omitempty"`
	Notifications  *bool                 `json:"notifications,omitempty"`
	Commands       map[string]string     `json:"commands,omitempty"`
	ResourceLimits *ResourceLimitsConfig `json:"resourceLimits,omitempty"`
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
	for _, dep := range a.DependsOn {
		if dep == "" {
			return fmt.Errorf("\"dependsOn\" entries must be non-empty strings")
		}
	}
	if a.HealthCheck != nil && a.HealthCheck.URL == "" {
		return fmt.Errorf("\"healthCheck.url\" must be non-empty when healthCheck is specified")
	}
	for k, v := range a.Commands {
		if v == "" {
			return fmt.Errorf("\"commands\" value for %q must be non-empty", k)
		}
	}
	if a.ResourceLimits != nil {
		if a.ResourceLimits.MaxCPU < 0 {
			return fmt.Errorf("\"resourceLimits.maxCpu\" must be non-negative")
		}
		if a.ResourceLimits.MaxMemoryMB < 0 {
			return fmt.Errorf("\"resourceLimits.maxMemoryMB\" must be non-negative")
		}
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
		if len(a.Env) > 0 {
			clean[i].Env = a.Env
		}
		if len(a.DependsOn) > 0 {
			clean[i].DependsOn = a.DependsOn
		}
		if a.Group != "" {
			clean[i].Group = a.Group
		}
		if a.HealthCheck != nil {
			clean[i].HealthCheck = a.HealthCheck
		}
		if a.Pinned != nil {
			clean[i].Pinned = a.Pinned
		}
		if a.Notifications != nil {
			clean[i].Notifications = a.Notifications
		}
		if len(a.Commands) > 0 {
			clean[i].Commands = a.Commands
		}
		if a.ResourceLimits != nil {
			clean[i].ResourceLimits = a.ResourceLimits
		}
	}

	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(configPath, data, 0644)
}

// HasChanged checks if two App entries differ in significant fields.
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
	if old.Group != new.Group {
		return true
	}
	if !mapsEqual(old.Env, new.Env) {
		return true
	}
	if !slicesEqual(old.DependsOn, new.DependsOn) {
		return true
	}
	if !mapsEqual(old.Commands, new.Commands) {
		return true
	}
	if !healthCheckEqual(old.HealthCheck, new.HealthCheck) {
		return true
	}
	return false
}

// ValidateDependencies checks that all DependsOn references exist.
func ValidateDependencies(apps []App) error {
	names := make(map[string]bool, len(apps))
	for _, a := range apps {
		names[a.Name] = true
	}
	for _, a := range apps {
		for _, dep := range a.DependsOn {
			if dep == a.Name {
				return fmt.Errorf("app %q depends on itself", a.Name)
			}
			if !names[dep] {
				return fmt.Errorf("app %q depends on unknown app %q", a.Name, dep)
			}
		}
	}
	return nil
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || v != bv {
			return false
		}
	}
	return true
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func healthCheckEqual(a, b *HealthCheckConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.URL == b.URL && a.Interval == b.Interval
}
