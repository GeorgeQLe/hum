package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/process"
)

// startAPIServer creates and starts the HTTP API server, wiring it to the model's data.
func (m *Model) startAPIServer() tea.Cmd {
	pm := m.procManager
	apps := &m.apps
	model := m

	deps := api.ServerDeps{
		GetApps: func() []api.AppInfo {
			var infos []api.AppInfo
			for _, app := range *apps {
				infos = append(infos, api.AppInfo{
					Name:      app.Name,
					Dir:       app.Dir,
					Command:   app.Command,
					Ports:     app.Ports,
					Status:    string(pm.GetStatus(app.Name)),
					PID:       pm.PID(app.Name),
					Project:   app.Project,
					Group:     app.Group,
					AutoStart: app.AutoStart,
				})
			}
			return infos
		},
		GetAppDetail: func(name string) *api.AppDetail {
			for _, app := range *apps {
				if app.Name == name {
					detail := &api.AppDetail{
						AppInfo: api.AppInfo{
							Name:      app.Name,
							Dir:       app.Dir,
							Command:   app.Command,
							Ports:     app.Ports,
							Status:    string(pm.GetStatus(app.Name)),
							PID:       pm.PID(app.Name),
							Project:   app.Project,
							Group:     app.Group,
							AutoStart: app.AutoStart,
						},
						Env:        app.Env,
						DependsOn:  app.DependsOn,
						ErrorCount: pm.GetErrorCount(app.Name),
					}
					if entry := pm.GetEntry(app.Name); entry != nil {
						entry.Cmd = nil // don't expose
						detail.RestartCount = entry.RestartCount
						detail.ExitCode = entry.ExitCode
					}
					if pm.GetStatus(app.Name) == process.StatusRunning {
						detail.Uptime = formatUptime(pm.Uptime(app.Name))
					}
					return detail
				}
			}
			return nil
		},
		GetLogs: func(name string, lines int) []api.LogEntry {
			buf := pm.GetLogBuffer(name)
			allLines, _, _ := buf.Snapshot()
			start := 0
			if len(allLines) > lines {
				start = len(allLines) - lines
			}
			var entries []api.LogEntry
			for _, l := range allLines[start:] {
				entries = append(entries, api.LogEntry{
					Text:      process.StripAnsi(l.Text),
					Timestamp: l.Timestamp,
					IsStderr:  l.IsStderr,
				})
			}
			return entries
		},
		GetErrors: func(name string) []api.ErrorEntry {
			eb := pm.GetErrorBuffer(name)
			if eb == nil {
				return nil
			}
			eb2 := eb // avoid lock issues — use public API
			var entries []api.ErrorEntry
			for _, e := range eb2.Errors {
				plainLines := make([]string, len(e.Lines))
				for j, l := range e.Lines {
					plainLines[j] = process.StripAnsi(l)
				}
				entries = append(entries, api.ErrorEntry{
					Timestamp: e.Timestamp,
					Lines:     plainLines,
					AppName:   e.AppName,
				})
			}
			return entries
		},
		GetPorts: func() []api.PortMapping {
			var mappings []api.PortMapping
			for _, app := range *apps {
				for _, p := range app.Ports {
					mappings = append(mappings, api.PortMapping{
						Port:    p,
						AppName: app.Name,
						Status:  string(pm.GetStatus(app.Name)),
					})
				}
			}
			return mappings
		},
		GetStats: func() []api.AppStats {
			var stats []api.AppStats
			for _, app := range *apps {
				s := api.AppStats{
					Name:   app.Name,
					Status: string(pm.GetStatus(app.Name)),
					PID:    pm.PID(app.Name),
				}
				if rs := model.resourceMonitor.GetStats(app.Name); rs != nil {
					s.CPU = rs.Current.CPUPercent
					s.MemRSS = rs.Current.MemoryRSS
					s.AvgCPU = rs.AvgCPU
					s.MaxCPU = rs.MaxCPU
					s.AvgMem = rs.AvgMemory
					s.MaxMem = rs.MaxMemory
					s.Samples = rs.SampleCount
				}
				if pm.GetStatus(app.Name) == process.StatusRunning {
					s.Uptime = formatUptime(pm.Uptime(app.Name))
				}
				stats = append(stats, s)
			}
			return stats
		},
		ApprovalQueue: m.approvalQueue,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) {
			return model.executeAPIAction(action, appName, payload)
		},
	}

	srv, err := api.NewServer(deps)
	if err != nil {
		m.systemLog(fmt.Sprintf("API server failed to start: %s", err))
		return nil
	}
	m.apiServer = srv
	m.systemLog(fmt.Sprintf("API server listening on 127.0.0.1:%d", srv.Port()))
	return nil
}

// executeAPIAction executes a mutating action from the HTTP API.
// This runs in the HTTP handler goroutine (after approval), so we use
// the process manager directly for thread-safe operations.
func (m *Model) executeAPIAction(action, appName string, payload []byte) (string, error) {
	switch action {
	case "register":
		return m.apiRegisterApp(payload)
	case "remove":
		return m.apiRemoveApp(appName)
	case "start":
		return m.apiStartApp(appName)
	case "stop":
		return m.apiStopApp(appName)
	case "restart":
		return m.apiRestartApp(appName)
	case "scan":
		return m.apiScanApps()
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (m *Model) apiRegisterApp(payload []byte) (string, error) {
	var entry struct {
		Name    string            `json:"name"`
		Dir     string            `json:"dir"`
		Command string            `json:"command"`
		Ports   []int             `json:"ports"`
		Project string            `json:"project,omitempty"`
		Env     map[string]string `json:"env,omitempty"`
	}
	if err := json.Unmarshal(payload, &entry); err != nil {
		return "", fmt.Errorf("invalid app data: %w", err)
	}

	// Check for duplicates
	for _, a := range m.apps {
		if a.Name == entry.Name {
			return "", fmt.Errorf("app %q already exists", entry.Name)
		}
	}

	// Auto-assign port if empty
	if len(entry.Ports) == 0 {
		var usedPorts []int
		for _, a := range m.apps {
			usedPorts = append(usedPorts, a.Ports...)
		}
		port := process.FindFreePort(usedPorts, 3000)
		if port == 0 {
			return "", fmt.Errorf("could not find a free port")
		}
		entry.Ports = []int{port}
		if entry.Env == nil {
			entry.Env = map[string]string{}
		}
		entry.Env["PORT"] = fmt.Sprintf("%d", port)
	}

	app := config.App{
		Name:    entry.Name,
		Dir:     entry.Dir,
		Command: entry.Command,
		Ports:   entry.Ports,
		Project: entry.Project,
		Env:     entry.Env,
	}
	if err := app.Validate(); err != nil {
		return "", fmt.Errorf("invalid app: %w", err)
	}

	m.apps = append(m.apps, app)
	m.saveConfig()
	m.systemLog(fmt.Sprintf("App \"%s\" registered via API (dir: %s)", entry.Name, entry.Dir))
	return fmt.Sprintf("registered app %q", entry.Name), nil
}

func (m *Model) apiRemoveApp(name string) (string, error) {
	idx := -1
	for i, a := range m.apps {
		if a.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", fmt.Errorf("app %q not found", name)
	}

	// Stop if running
	if m.procManager.GetStatus(name) == process.StatusRunning {
		m.procManager.Stop(name)
	}
	m.procManager.RemoveEntries(name)

	m.apps = append(m.apps[:idx], m.apps[idx+1:]...)
	m.saveConfig()
	m.systemLog(fmt.Sprintf("App \"%s\" removed via API", name))
	return fmt.Sprintf("removed app %q", name), nil
}

func (m *Model) apiStartApp(name string) (string, error) {
	if name == "all" {
		for _, app := range m.apps {
			if m.procManager.GetStatus(app.Name) != process.StatusRunning {
				go m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			}
		}
		return "starting all apps", nil
	}

	app := m.findApp(name)
	if app == nil {
		return "", fmt.Errorf("app %q not found", name)
	}
	status := m.procManager.GetStatus(name)
	if status == process.StatusRunning {
		return "", fmt.Errorf("%s is already running", name)
	}

	// Auto-resolve port conflicts
	cmd := app.Command
	for _, port := range app.Ports {
		if !process.IsPortFree(port) {
			altPort := process.SuggestAlternativePort(port)
			if altPort > 0 {
				m.systemLog(fmt.Sprintf("[api] Port %d in use, using %d for %s", port, altPort, name))
				cmd = fmt.Sprintf("PORT=%d %s", altPort, cmd)
			}
		}
	}
	go m.procManager.Start(app.Name, cmd, app.Dir, m.appEnv(app.Env, app.VaultEnv))
	return fmt.Sprintf("%s starting", name), nil
}

func (m *Model) apiStopApp(name string) (string, error) {
	if name == "all" {
		m.procManager.StopAll()
		return "stopping all apps", nil
	}

	app := m.findApp(name)
	if app == nil {
		return "", fmt.Errorf("app %q not found", name)
	}
	if m.procManager.GetStatus(name) != process.StatusRunning {
		return "", fmt.Errorf("%s is not running", name)
	}
	m.procManager.Stop(name)
	return fmt.Sprintf("%s stopping", name), nil
}

func (m *Model) apiRestartApp(name string) (string, error) {
	if name == "all" {
		for _, app := range m.apps {
			go m.procManager.Restart(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
		}
		return "restarting all apps", nil
	}

	app := m.findApp(name)
	if app == nil {
		return "", fmt.Errorf("app %q not found", name)
	}
	go m.procManager.Restart(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
	return fmt.Sprintf("%s restarting", name), nil
}

func (m *Model) apiScanApps() (string, error) {
	candidates := config.DetectApps(m.projectRoot, m.apps)
	if len(candidates) == 0 {
		return "no new apps detected", nil
	}

	var usedPorts []int
	for _, a := range m.apps {
		usedPorts = append(usedPorts, a.Ports...)
	}

	var added []string
	for _, c := range candidates {
		// Check for name collision
		exists := false
		for _, a := range m.apps {
			if a.Name == c.Name {
				exists = true
				break
			}
		}
		if exists {
			continue
		}

		ports := c.Ports
		var env map[string]string
		if len(ports) == 0 {
			port := process.FindFreePort(usedPorts, 3000)
			if port == 0 {
				continue
			}
			ports = []int{port}
			env = map[string]string{"PORT": fmt.Sprintf("%d", port)}
			usedPorts = append(usedPorts, port)
		}

		app := config.App{
			Name:    c.Name,
			Dir:     c.Dir,
			Command: c.Command,
			Ports:   ports,
			Env:     env,
		}
		if err := app.Validate(); err != nil {
			continue
		}

		m.apps = append(m.apps, app)
		added = append(added, c.Name)
	}

	if len(added) == 0 {
		return "no valid apps to add", nil
	}

	m.saveConfig()
	m.systemLog(fmt.Sprintf("Scan added %d app(s) via API: %s", len(added), strings.Join(added, ", ")))
	return fmt.Sprintf("added %d app(s): %s", len(added), strings.Join(added, ", ")), nil
}

// stopAPIServer gracefully stops the HTTP API server.
func (m *Model) stopAPIServer() {
	if m.apiServer != nil {
		m.apiServer.Stop()
		m.apiServer = nil
	}
	if m.approvalQueue != nil {
		m.approvalQueue.DenyAll()
	}
}
