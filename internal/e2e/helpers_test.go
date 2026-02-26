//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/health"
	"github.com/georgele/devctl/internal/process"
)

// TestEnv provides an isolated test environment with a process manager.
type TestEnv struct {
	T           *testing.T
	ProjectRoot string
	PM          *process.Manager
}

// NewTestEnv creates a new isolated test environment.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()
	root := t.TempDir()
	pm := process.NewManager(root)

	env := &TestEnv{
		T:           t,
		ProjectRoot: root,
		PM:          pm,
	}

	t.Cleanup(func() {
		pm.StopAll()
		drainAllEvents(pm, 2*time.Second)
	})

	return env
}

// WaitForStatus polls GetStatus at 20ms intervals until the expected status
// is reached or timeout expires.
func WaitForStatus(t *testing.T, pm *process.Manager, app string, want process.Status, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			got := pm.GetStatus(app)
			t.Fatalf("WaitForStatus(%s): timeout after %v, got %s want %s", app, timeout, got, want)
		case <-tick.C:
			if pm.GetStatus(app) == want {
				return
			}
		}
	}
}

// WaitForEvent reads from Events() until predicate returns true or timeout.
func WaitForEvent(t *testing.T, pm *process.Manager, timeout time.Duration, pred func(process.ProcessEvent) bool) process.ProcessEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("WaitForEvent: timeout after %v", timeout)
			return process.ProcessEvent{}
		case evt := <-pm.Events():
			if pred(evt) {
				return evt
			}
		}
	}
}

// WaitForEventType waits for a specific event type for a specific app.
func WaitForEventType(t *testing.T, pm *process.Manager, app string, evtType process.EventType, timeout time.Duration) process.ProcessEvent {
	t.Helper()
	return WaitForEvent(t, pm, timeout, func(e process.ProcessEvent) bool {
		return e.AppName == app && e.Type == evtType
	})
}

// WaitForLogOutput polls the log buffer until it contains the expected string.
func WaitForLogOutput(t *testing.T, pm *process.Manager, app string, contains string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-deadline:
			t.Fatalf("WaitForLogOutput(%s, %q): timeout after %v", app, contains, timeout)
		case <-tick.C:
			buf := pm.GetLogBuffer(app)
			snapshot, _, _ := buf.Snapshot()
			for _, line := range snapshot {
				if strings.Contains(line.Text, contains) {
					return
				}
			}
		}
	}
}

// drainAllEvents drains the event channel to prevent goroutine leaks.
func drainAllEvents(pm *process.Manager, timeout time.Duration) {
	deadline := time.After(timeout)
	for {
		select {
		case <-pm.Events():
		case <-deadline:
			return
		}
	}
}

// FreeTCPPort returns an available TCP port on localhost.
func FreeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("FreeTCPPort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// AppConfig describes an app for HeadlessSystem and IPC test environments.
type AppConfig struct {
	Name    string
	Command string
	Dir     string
	Ports   []int
	Env     map[string]string
}

// HeadlessSystem wires a real Manager, API Server, and Health Checker
// without a TUI, for integration testing.
type HeadlessSystem struct {
	T      *testing.T
	PM     *process.Manager
	API    *api.Server
	Health *health.Checker
	Root   string
	Apps   []AppConfig
}

// NewHeadlessSystem constructs and starts a headless system with the given apps.
func NewHeadlessSystem(t *testing.T, apps []AppConfig) *HeadlessSystem {
	t.Helper()

	root := t.TempDir()
	pm := process.NewManager(root)
	hc := health.NewChecker()

	// Use auto-approve for all actions so tests don't block on approval.
	cfg := api.ApprovalConfig{
		TimeoutSeconds: 5,
		Rules: map[string]api.ApprovalRule{
			"register": api.ApprovalAuto,
			"remove":   api.ApprovalAuto,
			"start":    api.ApprovalAuto,
			"stop":     api.ApprovalAuto,
			"restart":  api.ApprovalAuto,
			"scan":     api.ApprovalAuto,
		},
	}
	queue := api.NewApprovalQueue(cfg)

	sys := &HeadlessSystem{
		T:      t,
		PM:     pm,
		Health: hc,
		Root:   root,
		Apps:   apps,
	}

	deps := api.ServerDeps{
		GetApps: func() []api.AppInfo {
			var result []api.AppInfo
			for _, a := range sys.Apps {
				result = append(result, api.AppInfo{
					Name:    a.Name,
					Dir:     a.Dir,
					Command: a.Command,
					Ports:   a.Ports,
					Status:  string(pm.GetStatus(a.Name)),
					PID:     pm.PID(a.Name),
				})
			}
			return result
		},
		GetAppDetail: func(name string) *api.AppDetail {
			for _, a := range sys.Apps {
				if a.Name == name {
					entry := pm.GetEntry(a.Name)
					var restartCount, exitCode int
					if entry != nil {
						restartCount, exitCode = entry.GetDetail()
					}
					return &api.AppDetail{
						AppInfo: api.AppInfo{
							Name:    a.Name,
							Dir:     a.Dir,
							Command: a.Command,
							Ports:   a.Ports,
							Status:  string(pm.GetStatus(a.Name)),
							PID:     pm.PID(a.Name),
						},
						Uptime:       pm.Uptime(a.Name).String(),
						RestartCount: restartCount,
						ExitCode:     exitCode,
						Env:          a.Env,
						ErrorCount:   pm.GetErrorCount(a.Name),
					}
				}
			}
			return nil
		},
		GetLogs: func(name string, lines int) []api.LogEntry {
			buf := pm.GetLogBuffer(name)
			snapshot, _, _ := buf.Snapshot()
			start := 0
			if len(snapshot) > lines {
				start = len(snapshot) - lines
			}
			var result []api.LogEntry
			for _, l := range snapshot[start:] {
				result = append(result, api.LogEntry{
					Text:      l.Text,
					Timestamp: l.Timestamp,
					IsStderr:  l.IsStderr,
				})
			}
			return result
		},
		GetErrors: func(name string) []api.ErrorEntry {
			buf := pm.GetErrorBuffer(name)
			errs := buf.SnapshotErrors()
			var result []api.ErrorEntry
			for _, e := range errs {
				result = append(result, api.ErrorEntry{
					Timestamp: e.Timestamp,
					Lines:     e.Lines,
					AppName:   e.AppName,
				})
			}
			return result
		},
		GetPorts: func() []api.PortMapping {
			var result []api.PortMapping
			for _, a := range sys.Apps {
				for _, p := range a.Ports {
					result = append(result, api.PortMapping{
						Port:    p,
						AppName: a.Name,
						Status:  string(pm.GetStatus(a.Name)),
					})
				}
			}
			return result
		},
		GetStats: func() []api.AppStats {
			return nil
		},
		ApprovalQueue: queue,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) {
			switch action {
			case "start":
				for _, a := range sys.Apps {
					if a.Name == appName {
						if err := pm.Start(a.Name, a.Command, a.Dir, a.Env); err != nil {
							return "", err
						}
						return fmt.Sprintf("started %s", appName), nil
					}
				}
				return "", fmt.Errorf("app %q not found", appName)
			case "stop":
				if err := pm.Stop(appName); err != nil {
					return "", err
				}
				return fmt.Sprintf("stopped %s", appName), nil
			case "restart":
				for _, a := range sys.Apps {
					if a.Name == appName {
						if err := pm.Restart(a.Name, a.Command, a.Dir, a.Env); err != nil {
							return "", err
						}
						return fmt.Sprintf("restarted %s", appName), nil
					}
				}
				return "", fmt.Errorf("app %q not found", appName)
			default:
				return "", fmt.Errorf("unsupported action: %s", action)
			}
		},
	}

	srv, err := api.NewServer(deps)
	if err != nil {
		t.Fatalf("NewHeadlessSystem: api.NewServer: %v", err)
	}
	sys.API = srv

	t.Cleanup(func() {
		pm.StopAll()
		hc.StopAll()
		srv.Stop()
		drainAllEvents(pm, 2*time.Second)
	})

	return sys
}

// APIGet performs an authenticated GET request against the API.
func (s *HeadlessSystem) APIGet(path string) (*http.Response, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", s.API.Port(), path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.API.Token())
	return http.DefaultClient.Do(req)
}

// APIGetNoAuth performs an unauthenticated GET request.
func (s *HeadlessSystem) APIGetNoAuth(path string) (*http.Response, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", s.API.Port(), path)
	return http.Get(url)
}

// APIPost performs an authenticated POST request.
func (s *HeadlessSystem) APIPost(path string, body string) (*http.Response, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", s.API.Port(), path)
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.API.Token())
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

// ReadJSON reads and parses the response body as JSON into a map.
func ReadJSON(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("ReadJSON: unmarshal: %v\nbody: %s", err, string(body))
	}
	return result
}
