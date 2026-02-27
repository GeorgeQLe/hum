package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// PIDEntry tracks a managed process.
type PIDEntry struct {
	AppName   string    `json:"appName"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
}

// PIDFile represents ~/.humrun/pids.json.
type PIDFile struct {
	HumrunPID int        `json:"humrunPid"`
	Entries   []PIDEntry `json:"entries"`
}

func pidFilePath() string {
	return filepath.Join(GlobalDir(), "pids.json")
}

// WritePIDFile writes the PID tracking file.
func WritePIDFile(pf PIDFile) error {
	dir := GlobalDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := pidFilePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadPIDFile reads the PID tracking file.
func ReadPIDFile() (*PIDFile, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return nil, err
	}
	var pf PIDFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, err
	}
	return &pf, nil
}

// RemovePIDFile deletes the PID tracking file.
func RemovePIDFile() {
	os.Remove(pidFilePath())
}

// FindOrphanedProcesses reads the PID file and returns entries whose
// parent humrun process is no longer running.
func FindOrphanedProcesses() []PIDEntry {
	pf, err := ReadPIDFile()
	if err != nil {
		return nil
	}

	// Check if the humrun process that wrote this file is still alive
	if pf.HumrunPID > 0 && isProcessAlive(pf.HumrunPID) {
		// Previous instance is still running, no orphans
		return nil
	}

	// Previous humrun is dead — check which managed processes are still alive
	var orphans []PIDEntry
	for _, entry := range pf.Entries {
		if entry.PID > 0 && isProcessAlive(entry.PID) {
			orphans = append(orphans, entry)
		}
	}
	return orphans
}

// isProcessAlive checks if a process with the given PID exists.
func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// CleanupOrphans kills orphaned processes from a previous humrun instance.
// Returns the names of cleaned up processes.
func CleanupOrphans() []string {
	orphans := FindOrphanedProcesses()
	if len(orphans) == 0 {
		return nil
	}

	var cleaned []string
	for _, entry := range orphans {
		// Send SIGTERM to process group
		_ = syscall.Kill(-entry.PID, syscall.SIGTERM)
		cleaned = append(cleaned, fmt.Sprintf("%s (PID %d)", entry.AppName, entry.PID))
	}

	// Give processes a moment to exit, then force kill any remaining
	time.Sleep(2 * time.Second)
	for _, entry := range orphans {
		if isProcessAlive(entry.PID) {
			_ = syscall.Kill(-entry.PID, syscall.SIGKILL)
		}
	}

	// Remove stale PID file
	RemovePIDFile()
	return cleaned
}
