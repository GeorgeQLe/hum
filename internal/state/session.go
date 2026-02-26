package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const stateFile = ".devctl-state.json"

// StatePath returns the path to the session state file.
func StatePath(projectRoot string) string {
	return filepath.Join(projectRoot, stateFile)
}

// SaveSession persists the list of running app names.
func SaveSession(projectRoot string, runningApps []string) error {
	data, err := json.MarshalIndent(runningApps, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(StatePath(projectRoot), data, 0600)
}

// LoadSession reads the list of previously running app names.
func LoadSession(projectRoot string) []string {
	data, err := os.ReadFile(StatePath(projectRoot))
	if err != nil {
		return nil
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil
	}
	return names
}

// ClearSession removes the session state file.
func ClearSession(projectRoot string) {
	os.Remove(StatePath(projectRoot))
}
