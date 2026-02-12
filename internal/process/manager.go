package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Status represents the lifecycle state of a managed process.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusCrashed  Status = "crashed"

	stopTimeout     = 5 * time.Second
	eventChannelSize = 256
)

// Entry tracks a managed process and its metadata.
type Entry struct {
	Cmd                 *exec.Cmd
	Status              Status
	StartedAt           time.Time
	ExitCode            int
	RestartCount        int
	AutoRestartDisabled bool

	mu     sync.Mutex
	doneCh chan struct{} // closed when process exits
}

// GetAutoRestartState returns the auto-restart disabled flag and restart count.
func (e *Entry) GetAutoRestartState() (disabled bool, restartCount int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.AutoRestartDisabled, e.RestartCount
}

// ToggleAutoRestart toggles the auto-restart disabled flag and returns the new state.
func (e *Entry) ToggleAutoRestart() (nowDisabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AutoRestartDisabled = !e.AutoRestartDisabled
	return e.AutoRestartDisabled
}

// EnableAutoRestart enables auto-restart and resets the restart count.
func (e *Entry) EnableAutoRestart() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AutoRestartDisabled = false
	e.RestartCount = 0
}

// DisableAutoRestart disables auto-restart.
func (e *Entry) DisableAutoRestart() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.AutoRestartDisabled = true
}

// TryAutoRestart attempts to increment the restart counter.
// Returns whether a restart is allowed and the current count.
func (e *Entry) TryAutoRestart(maxRestarts int) (canRestart bool, count int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.AutoRestartDisabled {
		return false, e.RestartCount
	}
	if e.RestartCount >= maxRestarts {
		return false, e.RestartCount
	}
	e.RestartCount++
	return true, e.RestartCount
}

// ProcessEvent is sent when a process changes state.
type ProcessEvent struct {
	AppName string
	Type    EventType
	Message string
	Code    int
}

type EventType int

const (
	EventStarted EventType = iota
	EventStopped
	EventCrashed
	EventOutput
	EventStderrOutput
	EventError
	EventErrorDetected
)

// Manager handles spawning, stopping, and monitoring processes.
type Manager struct {
	ProjectRoot  string
	Entries      map[string]*Entry
	LogBuffers   map[string]*LogBuffer
	ErrorBuffers map[string]*ErrorBuffer
	mu           sync.Mutex
	eventCh      chan ProcessEvent
}

// NewManager creates a new process manager.
func NewManager(projectRoot string) *Manager {
	return &Manager{
		ProjectRoot:  projectRoot,
		Entries:      make(map[string]*Entry),
		LogBuffers:   make(map[string]*LogBuffer),
		ErrorBuffers: make(map[string]*ErrorBuffer),
		eventCh:      make(chan ProcessEvent, eventChannelSize),
	}
}

// Events returns the channel for process events.
func (m *Manager) Events() <-chan ProcessEvent {
	return m.eventCh
}

// GetLogBuffer returns the log buffer for an app, creating one if needed.
func (m *Manager) GetLogBuffer(name string) *LogBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf, ok := m.LogBuffers[name]
	if !ok {
		buf = NewLogBuffer()
		m.LogBuffers[name] = buf
	}
	return buf
}

// GetEntry returns the entry for an app, or nil.
func (m *Manager) GetEntry(name string) *Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Entries[name]
}

// GetStatus returns the status of an app.
func (m *Manager) GetStatus(name string) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.Entries[name]
	if !ok {
		return StatusStopped
	}
	return e.Status
}

// Start spawns a process for the given app.
// env optionally provides extra environment variables for the process.
func (m *Manager) Start(name, command, dir string, env map[string]string) error {
	m.mu.Lock()
	existing, hasExisting := m.Entries[name]
	if hasExisting {
		if existing.Status == StatusRunning {
			m.mu.Unlock()
			return fmt.Errorf("%s is already running", name)
		}
		if existing.Status == StatusStopping {
			m.mu.Unlock()
			return fmt.Errorf("%s is still stopping", name)
		}
	}
	m.mu.Unlock()

	fullDir := dir
	if !filepath.IsAbs(dir) {
		fullDir = filepath.Join(m.ProjectRoot, dir)
	}

	if _, err := os.Stat(fullDir); err != nil {
		buf := m.GetLogBuffer(name)
		buf.Append(fmt.Sprintf("Warning: directory does not exist: %s", fullDir), false)
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = fullDir
	cmd.Env = append(os.Environ(), "TURBO_UI=stream")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	// Explicitly ensure child reads from /dev/null, not the terminal (A3)
	cmd.Stdin = nil
	// Set process group so we can kill the entire group
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		m.sendEvent(ProcessEvent{
			AppName: name,
			Type:    EventError,
			Message: fmt.Sprintf("failed to start: %s", err),
		})
		return err
	}

	entry := &Entry{
		Cmd:       cmd,
		Status:    StatusRunning,
		StartedAt: time.Now(),
		doneCh:    make(chan struct{}),
	}

	// Preserve restart count from previous entry
	if hasExisting {
		entry.RestartCount = existing.RestartCount
		entry.AutoRestartDisabled = existing.AutoRestartDisabled
	}

	m.mu.Lock()
	m.Entries[name] = entry
	m.mu.Unlock()

	buf := m.GetLogBuffer(name)
	buf.Append(fmt.Sprintf("Started %s (PID %d)", name, cmd.Process.Pid), false)

	m.sendEvent(ProcessEvent{
		AppName: name,
		Type:    EventStarted,
		Message: fmt.Sprintf("Started %s (PID %d)", name, cmd.Process.Pid),
	})

	// Read stdout in background
	go m.readOutput(name, stdout, false)
	// Read stderr in background
	go m.readOutput(name, stderr, true)

	// Wait for process exit in background
	go func() {
		// Ensure doneCh is always closed so Stop() never hangs.
		defer func() {
			if r := recover(); r != nil {
				entry.mu.Lock()
				if entry.Status == StatusRunning || entry.Status == StatusStopping {
					entry.Status = StatusCrashed
				}
				entry.Cmd = nil
				entry.mu.Unlock()
				fmt.Fprintf(os.Stderr, "devctl: panic in process wait goroutine for %s: %v\n", name, r)
			}
			close(entry.doneCh)
		}()

		err := cmd.Wait()
		entry.mu.Lock()
		wasStopping := entry.Status == StatusStopping
		if wasStopping {
			entry.Status = StatusStopped
			entry.RestartCount = 0
		} else {
			entry.Status = StatusCrashed
			if exitErr, ok := err.(*exec.ExitError); ok {
				entry.ExitCode = exitErr.ExitCode()
			}
		}
		entry.Cmd = nil
		entry.mu.Unlock()

		if wasStopping {
			buf := m.GetLogBuffer(name)
			buf.Append(fmt.Sprintf("Stopped %s.", name), false)
			m.sendEvent(ProcessEvent{
				AppName: name,
				Type:    EventStopped,
				Message: fmt.Sprintf("Stopped %s.", name),
			})
		} else {
			exitCode := -1
			if entry.ExitCode != 0 {
				exitCode = entry.ExitCode
			}
			buf := m.GetLogBuffer(name)
			buf.Append(fmt.Sprintf("[%s] exited (code=%d)", name, exitCode), false)
			m.sendEvent(ProcessEvent{
				AppName: name,
				Type:    EventCrashed,
				Message: fmt.Sprintf("[%s] exited (code=%d)", name, exitCode),
				Code:    exitCode,
			})
		}
	}()

	return nil
}

// Stop sends SIGTERM to the process group, then SIGKILL after timeout.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	entry, ok := m.Entries[name]
	m.mu.Unlock()

	if !ok {
		return nil
	}

	entry.mu.Lock()
	if entry.Status != StatusRunning {
		entry.mu.Unlock()
		return nil
	}
	entry.Status = StatusStopping
	cmd := entry.Cmd
	entry.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid

	// Send SIGTERM to process group
	_ = syscall.Kill(-pid, syscall.SIGTERM)

	// Wait for exit or timeout
	select {
	case <-entry.doneCh:
		return nil
	case <-time.After(stopTimeout):
		// Force kill
		buf := m.GetLogBuffer(name)
		buf.Append(fmt.Sprintf("[%s] SIGTERM timeout, sending SIGKILL...", name), false)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-entry.doneCh
		return nil
	}
}

// Restart stops and then starts a process.
func (m *Manager) Restart(name, command, dir string, env map[string]string) error {
	if err := m.Stop(name); err != nil {
		return err
	}
	return m.Start(name, command, dir, env)
}

// StopAll stops all running processes.
func (m *Manager) StopAll() {
	m.mu.Lock()
	var running []string
	for name, e := range m.Entries {
		if e.Status == StatusRunning {
			running = append(running, name)
		}
	}
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, name := range running {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			m.Stop(n)
		}(name)
	}
	wg.Wait()
}

// PID returns the PID of the running process, or 0.
func (m *Manager) PID(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.Entries[name]
	if !ok || e.Cmd == nil || e.Cmd.Process == nil {
		return 0
	}
	return e.Cmd.Process.Pid
}

// Uptime returns the duration since the process was started.
func (m *Manager) Uptime(name string) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.Entries[name]
	if !ok || e.Status != StatusRunning {
		return 0
	}
	return time.Since(e.StartedAt)
}

// GetErrorBuffer returns the error buffer for an app, creating one if needed.
func (m *Manager) GetErrorBuffer(name string) *ErrorBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf, ok := m.ErrorBuffers[name]
	if !ok {
		buf = &ErrorBuffer{}
		m.ErrorBuffers[name] = buf
	}
	return buf
}

// GetErrorCount returns the number of captured errors for an app.
func (m *Manager) GetErrorCount(name string) int {
	m.mu.Lock()
	buf, ok := m.ErrorBuffers[name]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	return buf.Count()
}

// ClearErrors clears captured errors for an app.
func (m *Manager) ClearErrors(name string) {
	m.mu.Lock()
	buf, ok := m.ErrorBuffers[name]
	m.mu.Unlock()
	if ok {
		buf.Clear()
	}
}

// ClearAllErrors clears captured errors for all apps.
func (m *Manager) ClearAllErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, buf := range m.ErrorBuffers {
		buf.Clear()
	}
}

// RemoveEntries removes all state (entry, log buffer, error buffer) for an app.
func (m *Manager) RemoveEntries(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Entries, name)
	delete(m.LogBuffers, name)
	delete(m.ErrorBuffers, name)
}

// FindDevctlOwner returns the name of a devctl-managed app by PID, or "".
func (m *Manager) FindDevctlOwner(pid int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, e := range m.Entries {
		if e.Status == StatusRunning && e.Cmd != nil && e.Cmd.Process != nil && e.Cmd.Process.Pid == pid {
			return name
		}
	}
	return ""
}

// KillExternalProcess sends SIGTERM then SIGKILL to an external process
// and its process group to clean up child processes.
func KillExternalProcess(pid int) error {
	// Snapshot the command name before killing so we can verify PID identity later.
	origCmd := getProcessCommand(pid)

	// Try to kill the entire process group first
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		// Not a group leader or permission denied — fall back to single PID
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			return err
		}
	}

	// Wait up to 2.5s for exit, then SIGKILL
	for i := 0; i < 10; i++ {
		time.Sleep(250 * time.Millisecond)
		if err := syscall.Kill(pid, 0); err != nil {
			return nil // Process exited
		}
	}

	// Before sending SIGKILL, verify the PID still belongs to the same process.
	// Between our last check and now, the process could have exited and the PID
	// could have been reused by an unrelated process.
	if origCmd != "" {
		currentCmd := getProcessCommand(pid)
		if currentCmd != origCmd {
			// PID was reused by a different process — do not kill it.
			return nil
		}
	}

	// Force kill process group, then single PID as fallback
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	time.Sleep(500 * time.Millisecond)
	return nil
}

// getProcessCommand returns the command name for a PID using ps, or "" on failure.
func getProcessCommand(pid int) string {
	out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m *Manager) readOutput(name string, r interface{ Read([]byte) (int, error) }, isStderr bool) {
	buf := make([]byte, 4096)
	logBuf := m.GetLogBuffer(name)
	evtType := EventOutput
	if isStderr {
		evtType = EventStderrOutput
	}

	errBuf := m.GetErrorBuffer(name)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			text := string(buf[:n])
			indices := logBuf.Append(text, isStderr)
			m.sendEvent(ProcessEvent{
				AppName: name,
				Type:    evtType,
				Message: text,
			})

			// Check new lines for error patterns
			for _, idx := range indices {
				if line, ok := logBuf.GetLine(idx); ok && MatchesErrorPattern(line.Text) {
					errBuf.CaptureError(logBuf, idx)
					m.sendEvent(ProcessEvent{
						AppName: name,
						Type:    EventErrorDetected,
						Message: "Error detected",
					})
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) sendEvent(evt ProcessEvent) {
	select {
	case m.eventCh <- evt:
	default:
		// Channel full — use a short timeout so critical events (crashes, errors)
		// have a better chance of being delivered.
		if evt.Type == EventCrashed || evt.Type == EventErrorDetected || evt.Type == EventError {
			select {
			case m.eventCh <- evt:
			case <-time.After(100 * time.Millisecond):
				// Truly full; log to stderr as last resort
				fmt.Fprintf(os.Stderr, "devctl: event channel full, dropped %s event for %s\n", eventTypeName(evt.Type), evt.AppName)
			}
		}
	}
}

func eventTypeName(t EventType) string {
	switch t {
	case EventStarted:
		return "started"
	case EventStopped:
		return "stopped"
	case EventCrashed:
		return "crashed"
	case EventOutput:
		return "output"
	case EventStderrOutput:
		return "stderr"
	case EventError:
		return "error"
	case EventErrorDetected:
		return "error-detected"
	default:
		return "unknown"
	}
}
