package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ResourceUsage holds CPU and memory stats for a process.
type ResourceUsage struct {
	CPUPercent float64 // CPU usage percentage
	MemoryRSS int64   // Resident set size in bytes
}

// GetResourceUsage returns CPU and memory usage for a process by PID.
// Uses ps on macOS/Linux. Returns nil if the process is not found.
func GetResourceUsage(pid int) (*ResourceUsage, error) {
	out, err := exec.Command("ps", "-o", "%cpu=,rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return nil, fmt.Errorf("ps failed for PID %d: %w", pid, err)
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return nil, fmt.Errorf("unexpected ps output for PID %d", pid)
	}

	cpu, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("parse CPU: %w", err)
	}

	rssKB, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse RSS: %w", err)
	}

	return &ResourceUsage{
		CPUPercent: cpu,
		MemoryRSS:  rssKB * 1024, // ps reports in KB
	}, nil
}

// FormatMemory formats bytes into a human-readable string.
func FormatMemory(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.0fK", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1fM", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1fG", float64(bytes)/(1024*1024*1024))
}
