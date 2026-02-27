//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/georgele/hum/internal/process"
)

func TestStartSingleApp(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	err := pm.Start("ticker", `sh -c "echo started; while true; do echo tick; sleep 0.5; done"`, ".", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the started event.
	WaitForEventType(t, pm, "ticker", process.EventStarted, 5*time.Second)

	// Verify status and PID.
	if got := pm.GetStatus("ticker"); got != process.StatusRunning {
		t.Errorf("status: got %s, want running", got)
	}
	if pid := pm.PID("ticker"); pid <= 0 {
		t.Errorf("PID: got %d, want > 0", pid)
	}

	// Verify log buffer captured output.
	WaitForLogOutput(t, pm, "ticker", "started", 5*time.Second)

	// Stop and verify clean shutdown.
	if err := pm.Stop("ticker"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := pm.GetStatus("ticker"); got != process.StatusStopped {
		t.Errorf("after stop: got %s, want stopped", got)
	}
}

func TestStopAll(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	if err := pm.Start("a", "sleep 30", ".", nil); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	if err := pm.Start("b", "sleep 30", ".", nil); err != nil {
		t.Fatalf("Start b: %v", err)
	}

	WaitForStatus(t, pm, "a", process.StatusRunning, 5*time.Second)
	WaitForStatus(t, pm, "b", process.StatusRunning, 5*time.Second)

	pm.StopAll()

	// Both should be stopped (StopAll is synchronous).
	if got := pm.GetStatus("a"); got != process.StatusStopped {
		t.Errorf("a: got %s, want stopped", got)
	}
	if got := pm.GetStatus("b"); got != process.StatusStopped {
		t.Errorf("b: got %s, want stopped", got)
	}
}

func TestCrashDetection(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	err := pm.Start("crasher", `sh -c "sleep 0.3; exit 1"`, ".", nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	evt := WaitForEventType(t, pm, "crasher", process.EventCrashed, 10*time.Second)
	if evt.Code != 1 {
		t.Errorf("exit code: got %d, want 1", evt.Code)
	}
	if got := pm.GetStatus("crasher"); got != process.StatusCrashed {
		t.Errorf("status: got %s, want crashed", got)
	}
}

func TestRestart(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	if err := pm.Start("svc", "sleep 30", ".", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	WaitForStatus(t, pm, "svc", process.StatusRunning, 5*time.Second)
	pid1 := pm.PID("svc")

	if err := pm.Restart("svc", "sleep 30", ".", nil); err != nil {
		t.Fatalf("Restart: %v", err)
	}

	// After Restart returns, the new process should be running.
	if got := pm.GetStatus("svc"); got != process.StatusRunning {
		t.Errorf("status after restart: got %s, want running", got)
	}
	pid2 := pm.PID("svc")
	if pid1 == pid2 {
		t.Errorf("PID did not change after restart: %d", pid1)
	}
	if pid2 <= 0 {
		t.Errorf("new PID: got %d, want > 0", pid2)
	}
}

func TestErrorDetection(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	cmd := `sh -c "echo 'TypeError: undefined is not a function'; echo '  at Module._compile (internal/modules/cjs/loader.js:1063:30)'; sleep 30"`
	if err := pm.Start("errapp", cmd, ".", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	WaitForEventType(t, pm, "errapp", process.EventErrorDetected, 10*time.Second)

	buf := pm.GetErrorBuffer("errapp")
	if buf.Count() == 0 {
		t.Error("error buffer is empty after EventErrorDetected")
	}
}

func TestProcessGroupKill(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	// Start a shell that spawns a background child and waits for it.
	// Without process group kill, the child would keep running.
	if err := pm.Start("pgkill", `sh -c "sleep 60 & wait"`, ".", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	WaitForStatus(t, pm, "pgkill", process.StatusRunning, 5*time.Second)

	start := time.Now()
	if err := pm.Stop("pgkill"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	elapsed := time.Since(start)

	// Stop should complete well within the 5s SIGTERM timeout + 5s SIGKILL timeout.
	if elapsed > 10*time.Second {
		t.Errorf("Stop took %v, expected < 10s (process group kill may have failed)", elapsed)
	}
	if got := pm.GetStatus("pgkill"); got != process.StatusStopped {
		t.Errorf("status: got %s, want stopped", got)
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	if err := pm.Start("dup", "sleep 30", ".", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	WaitForStatus(t, pm, "dup", process.StatusRunning, 5*time.Second)

	err := pm.Start("dup", "sleep 30", ".", nil)
	if err == nil {
		t.Fatal("expected error from second Start, got nil")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEnvVars(t *testing.T) {
	env := NewTestEnv(t)
	pm := env.PM

	appEnv := map[string]string{"FOO": "bar"}
	if err := pm.Start("envtest", `sh -c "echo FOO=$FOO"`, ".", appEnv); err != nil {
		t.Fatalf("Start: %v", err)
	}

	WaitForLogOutput(t, pm, "envtest", "FOO=bar", 5*time.Second)
}
