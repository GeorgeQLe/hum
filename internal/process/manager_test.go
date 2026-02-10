package process

import (
	"runtime"
	"testing"
	"time"
)

func TestManagerStartStop(t *testing.T) {
	m := NewManager(t.TempDir())

	err := m.Start("echo-app", "echo hello", ".")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for process to finish
	time.Sleep(500 * time.Millisecond)

	// Drain events
	drainEvents(m, 500*time.Millisecond)

	// Check log buffer has output
	buf := m.GetLogBuffer("echo-app")
	if buf.LineCount() == 0 {
		t.Error("expected log output from echo")
	}
}

func TestManagerStartAlreadyRunning(t *testing.T) {
	m := NewManager(t.TempDir())

	err := m.Start("sleep-app", "sleep 10", ".")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop("sleep-app")

	// Wait for it to be running
	time.Sleep(200 * time.Millisecond)

	// Try to start again
	err = m.Start("sleep-app", "sleep 10", ".")
	if err == nil {
		t.Error("expected error when starting already running app")
	}
}

func TestManagerStopNotRunning(t *testing.T) {
	m := NewManager(t.TempDir())

	// Stop a non-existent app should not error
	err := m.Stop("nonexistent")
	if err != nil {
		t.Errorf("Stop non-existent: %v", err)
	}
}

func TestManagerRestart(t *testing.T) {
	m := NewManager(t.TempDir())

	err := m.Start("sleep-app", "sleep 10", ".")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Get the original PID
	pid1 := m.PID("sleep-app")
	if pid1 == 0 {
		t.Fatal("expected non-zero PID after start")
	}

	// Restart
	err = m.Restart("sleep-app", "sleep 10", ".")
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	defer m.Stop("sleep-app")

	time.Sleep(200 * time.Millisecond)

	// PID should have changed
	pid2 := m.PID("sleep-app")
	if pid2 == 0 {
		t.Fatal("expected non-zero PID after restart")
	}
	if pid1 == pid2 {
		t.Error("expected different PID after restart")
	}
}

func TestManagerStopAll(t *testing.T) {
	m := NewManager(t.TempDir())

	m.Start("app1", "sleep 10", ".")
	m.Start("app2", "sleep 10", ".")

	time.Sleep(200 * time.Millisecond)

	if m.GetStatus("app1") != StatusRunning {
		t.Error("app1 should be running")
	}
	if m.GetStatus("app2") != StatusRunning {
		t.Error("app2 should be running")
	}

	m.StopAll()

	// Wait for processes to exit
	time.Sleep(500 * time.Millisecond)

	if m.GetStatus("app1") == StatusRunning {
		t.Error("app1 should not be running after StopAll")
	}
	if m.GetStatus("app2") == StatusRunning {
		t.Error("app2 should not be running after StopAll")
	}
}

func TestManagerStatusTransitions(t *testing.T) {
	m := NewManager(t.TempDir())

	// Initially stopped
	if m.GetStatus("app") != StatusStopped {
		t.Errorf("expected stopped, got %s", m.GetStatus("app"))
	}

	m.Start("app", "sleep 10", ".")
	time.Sleep(200 * time.Millisecond)

	if m.GetStatus("app") != StatusRunning {
		t.Errorf("expected running, got %s", m.GetStatus("app"))
	}

	m.Stop("app")
	time.Sleep(500 * time.Millisecond)

	status := m.GetStatus("app")
	if status != StatusStopped {
		t.Errorf("expected stopped after stop, got %s", status)
	}
}

func TestManagerCrashDetection(t *testing.T) {
	m := NewManager(t.TempDir())

	// Start a command that exits with error
	m.Start("crash-app", "exit 1", ".")

	// Wait for crash
	time.Sleep(500 * time.Millisecond)
	drainEvents(m, 500*time.Millisecond)

	status := m.GetStatus("crash-app")
	if status != StatusCrashed {
		t.Errorf("expected crashed, got %s", status)
	}
}

func TestManagerUptime(t *testing.T) {
	m := NewManager(t.TempDir())

	// No uptime for non-existent
	if m.Uptime("nonexistent") != 0 {
		t.Error("expected 0 uptime for non-existent app")
	}

	m.Start("app", "sleep 10", ".")
	defer m.Stop("app")
	time.Sleep(200 * time.Millisecond)

	uptime := m.Uptime("app")
	if uptime < 100*time.Millisecond {
		t.Errorf("expected uptime > 100ms, got %v", uptime)
	}
}

func TestManagerRemoveEntries(t *testing.T) {
	m := NewManager(t.TempDir())

	m.Start("app", "echo done", ".")
	time.Sleep(300 * time.Millisecond)

	// Ensure entries exist
	if m.GetLogBuffer("app").LineCount() == 0 {
		t.Error("expected log entries")
	}

	m.RemoveEntries("app")

	if m.GetEntry("app") != nil {
		t.Error("expected nil entry after remove")
	}
}

func TestManagerGroupKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups not supported on Windows")
	}

	m := NewManager(t.TempDir())

	// Start a shell that spawns a child
	m.Start("parent", "sh -c 'sleep 30 & wait'", ".")
	time.Sleep(300 * time.Millisecond)

	if m.GetStatus("parent") != StatusRunning {
		t.Fatal("expected parent to be running")
	}

	m.Stop("parent")
	time.Sleep(1 * time.Second)

	if m.GetStatus("parent") == StatusRunning {
		t.Error("expected parent to be stopped")
	}
}

func TestManagerEvents(t *testing.T) {
	m := NewManager(t.TempDir())

	m.Start("app", "echo test", ".")

	// Should receive started event
	var gotStarted bool
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt := <-m.Events():
			if evt.AppName == "app" && evt.Type == EventStarted {
				gotStarted = true
			}
			if gotStarted {
				return
			}
		case <-timeout:
			if !gotStarted {
				t.Error("did not receive EventStarted")
			}
			return
		}
	}
}

func drainEvents(m *Manager, timeout time.Duration) {
	deadline := time.After(timeout)
	for {
		select {
		case <-m.Events():
		case <-deadline:
			return
		}
	}
}
