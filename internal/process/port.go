package process

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PortOwnerInfo describes the process listening on a port.
type PortOwnerInfo struct {
	Command string
	PID     int
	User    string
}

// IsPortFree checks if a TCP port is available on localhost.
func IsPortFree(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// GetPortOwnerInfo uses lsof to identify what process is listening on a port.
// Returns nil if the owner cannot be determined.
func GetPortOwnerInfo(port int) *PortOwnerInfo {
	ctx := fmt.Sprintf(":%d", port)
	cmd := exec.Command("lsof", "-i", ctx, "-P", "-n", "-sTCP:LISTEN")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil
	}

	// Parse: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
	parts := strings.Fields(lines[1])
	if len(parts) < 3 {
		return nil
	}

	pid, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}

	return &PortOwnerInfo{
		Command: parts[0],
		PID:     pid,
		User:    parts[2],
	}
}

// SuggestAlternativePort finds a free port near the given base port.
// Tries offsets of +1, +10, +100.
func SuggestAlternativePort(basePort int) int {
	offsets := []int{1, 10, 100}
	for _, offset := range offsets {
		candidate := basePort + offset
		if candidate < 65536 && IsPortFree(candidate) {
			return candidate
		}
	}
	return 0
}

// FindFreePort scans upward from basePort to find an available port,
// skipping ports in usedPorts and checking OS availability.
func FindFreePort(usedPorts []int, basePort int) int {
	used := make(map[int]bool, len(usedPorts))
	for _, p := range usedPorts {
		used[p] = true
	}
	for port := basePort; port <= 65535; port++ {
		if used[port] {
			continue
		}
		if IsPortFree(port) {
			return port
		}
	}
	return 0
}

// WaitForPortFree waits up to timeout for a port to become free.
func WaitForPortFree(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsPortFree(port) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
