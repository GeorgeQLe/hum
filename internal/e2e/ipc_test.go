//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/georgele/hum/internal/ipc"
	"github.com/georgele/hum/internal/process"
)

// ipcTestEnv holds the components for IPC integration tests.
type ipcTestEnv struct {
	T      *testing.T
	PM     *process.Manager
	Server *ipc.Server
	Client *ipc.Client
	Root   string
	Apps   []AppConfig
}

// newIPCTestEnv creates an IPC server, dispatcher, and client for testing.
func newIPCTestEnv(t *testing.T, apps []AppConfig) *ipcTestEnv {
	t.Helper()

	root := t.TempDir()
	pm := process.NewManager(root)

	ipcServer, err := ipc.NewServer(root)
	if err != nil {
		t.Fatalf("ipc.NewServer: %v", err)
	}
	ipcServer.Start()

	env := &ipcTestEnv{
		T:      t,
		PM:     pm,
		Server: ipcServer,
		Client: ipc.NewClient(root),
		Root:   root,
		Apps:   apps,
	}

	// Dispatcher goroutine: simulates the TUI's IPC handler.
	go func() {
		for reqMsg := range ipcServer.Requests() {
			env.dispatch(reqMsg)
		}
	}()

	// Give the server a moment to start accepting.
	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() {
		pm.StopAll()
		ipcServer.Stop()
		drainAllEvents(pm, 2*time.Second)
	})

	return env
}

func (e *ipcTestEnv) dispatch(reqMsg ipc.IPCRequestMsg) {
	switch reqMsg.Request.Action {
	case "ping":
		reqMsg.ResponseCh <- ipc.Response{OK: true, Message: "pong"}

	case "start":
		name := reqMsg.Request.Target
		for _, a := range e.Apps {
			if a.Name == name {
				if err := e.PM.Start(a.Name, a.Command, a.Dir, a.Env); err != nil {
					reqMsg.ResponseCh <- ipc.Response{OK: false, Error: err.Error()}
				} else {
					reqMsg.ResponseCh <- ipc.Response{OK: true, Message: "started " + name}
				}
				return
			}
		}
		reqMsg.ResponseCh <- ipc.Response{OK: false, Error: "app not found"}

	case "stop":
		name := reqMsg.Request.Target
		if err := e.PM.Stop(name); err != nil {
			reqMsg.ResponseCh <- ipc.Response{OK: false, Error: err.Error()}
		} else {
			reqMsg.ResponseCh <- ipc.Response{OK: true, Message: "stopped " + name}
		}

	case "status":
		type appStatus struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		}
		var statuses []appStatus
		for _, a := range e.Apps {
			statuses = append(statuses, appStatus{
				Name:   a.Name,
				Status: string(e.PM.GetStatus(a.Name)),
			})
		}
		data, _ := json.Marshal(statuses)
		reqMsg.ResponseCh <- ipc.Response{OK: true, Apps: data}

	default:
		reqMsg.ResponseCh <- ipc.Response{OK: false, Error: "unknown action"}
	}
}

func TestIPCPing(t *testing.T) {
	env := newIPCTestEnv(t, nil)

	resp, err := env.Client.Ping()
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !resp.OK {
		t.Errorf("Ping: OK is false, error: %s", resp.Error)
	}
	if resp.Message != "pong" {
		t.Errorf("Ping message: got %q, want pong", resp.Message)
	}
}

func TestIPCStartStop(t *testing.T) {
	apps := []AppConfig{
		{Name: "worker", Command: "sleep 30", Dir: "."},
	}
	env := newIPCTestEnv(t, apps)

	// Start via IPC.
	resp, err := env.Client.StartApp("worker")
	if err != nil {
		t.Fatalf("StartApp: %v", err)
	}
	if !resp.OK {
		t.Fatalf("StartApp: OK is false, error: %s", resp.Error)
	}

	WaitForStatus(t, env.PM, "worker", process.StatusRunning, 5*time.Second)

	// Stop via IPC.
	resp, err = env.Client.StopApp("worker")
	if err != nil {
		t.Fatalf("StopApp: %v", err)
	}
	if !resp.OK {
		t.Fatalf("StopApp: OK is false, error: %s", resp.Error)
	}

	WaitForStatus(t, env.PM, "worker", process.StatusStopped, 10*time.Second)
}

func TestIPCStatus(t *testing.T) {
	apps := []AppConfig{
		{Name: "svc-a", Command: "sleep 30", Dir: "."},
		{Name: "svc-b", Command: "sleep 30", Dir: "."},
	}
	env := newIPCTestEnv(t, apps)

	// Start both apps via IPC.
	for _, a := range apps {
		resp, err := env.Client.StartApp(a.Name)
		if err != nil {
			t.Fatalf("StartApp(%s): %v", a.Name, err)
		}
		if !resp.OK {
			t.Fatalf("StartApp(%s): error: %s", a.Name, resp.Error)
		}
	}

	for _, a := range apps {
		WaitForStatus(t, env.PM, a.Name, process.StatusRunning, 5*time.Second)
	}

	// Query status via IPC.
	resp, err := env.Client.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !resp.OK {
		t.Fatalf("Status: OK is false, error: %s", resp.Error)
	}

	var statuses []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp.Apps, &statuses); err != nil {
		t.Fatalf("unmarshal apps: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Status != "running" {
			t.Errorf("app %s: got status %q, want running", s.Name, s.Status)
		}
	}
}
