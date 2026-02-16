package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/ipc"
	"github.com/georgele/devctl/internal/process"
)

func (m Model) listenForIPCRequests() tea.Cmd {
	if m.ipcServer == nil {
		return nil
	}
	ch := m.ipcServer.Requests()
	return func() tea.Msg {
		req := <-ch
		return ipcRequestMsg(req)
	}
}

func (m *Model) handleIPCRequest(msg ipc.IPCRequestMsg) {
	switch msg.Request.Action {
	case "ping":
		msg.ResponseCh <- ipc.Response{
			OK:      true,
			PID:     os.Getpid(),
			Project: m.projectRoot,
		}

	case "status":
		type appStatus struct {
			Name   string `json:"name"`
			Dir    string `json:"dir"`
			Ports  []int  `json:"ports"`
			Status string `json:"status"`
			PID    int    `json:"pid,omitempty"`
		}
		var statuses []appStatus
		for _, app := range m.apps {
			status := string(m.procManager.GetStatus(app.Name))
			pid := m.procManager.PID(app.Name)
			statuses = append(statuses, appStatus{
				Name:   app.Name,
				Dir:    app.Dir,
				Ports:  app.Ports,
				Status: status,
				PID:    pid,
			})
		}
		appsJSON, _ := json.Marshal(statuses)
		msg.ResponseCh <- ipc.Response{
			OK:      true,
			PID:     os.Getpid(),
			Project: m.projectRoot,
			Apps:    appsJSON,
		}

	case "add-app":
		m.handleIPCAddApp(msg)

	case "stats":
		type appStats struct {
			Name    string  `json:"name"`
			Status  string  `json:"status"`
			PID     int     `json:"pid,omitempty"`
			CPU     float64 `json:"cpu"`
			MemRSS  int64   `json:"memRss"`
			AvgCPU  float64 `json:"avgCpu,omitempty"`
			MaxCPU  float64 `json:"maxCpu,omitempty"`
			AvgMem  int64   `json:"avgMem,omitempty"`
			MaxMem  int64   `json:"maxMem,omitempty"`
			Uptime  string  `json:"uptime,omitempty"`
			Samples int     `json:"samples,omitempty"`
		}
		var stats []appStats
		for _, app := range m.apps {
			s := appStats{
				Name:   app.Name,
				Status: string(m.procManager.GetStatus(app.Name)),
				PID:    m.procManager.PID(app.Name),
			}
			if rs := m.resourceMonitor.GetStats(app.Name); rs != nil {
				s.CPU = rs.Current.CPUPercent
				s.MemRSS = rs.Current.MemoryRSS
				s.AvgCPU = rs.AvgCPU
				s.MaxCPU = rs.MaxCPU
				s.AvgMem = rs.AvgMemory
				s.MaxMem = rs.MaxMemory
				s.Samples = rs.SampleCount
			}
			if m.procManager.GetStatus(app.Name) == process.StatusRunning {
				s.Uptime = formatUptime(m.procManager.Uptime(app.Name))
			}
			stats = append(stats, s)
		}
		appsJSON, _ := json.Marshal(stats)
		msg.ResponseCh <- ipc.Response{
			OK:      true,
			PID:     os.Getpid(),
			Project: m.projectRoot,
			Apps:    appsJSON,
		}

	case "build-error":
		errMsg := msg.Request.Message
		if errMsg != "" {
			m.systemLog("[dev] Build error:")
			for _, line := range strings.Split(errMsg, "\n") {
				m.systemLog("  " + line)
			}
		}
		msg.ResponseCh <- ipc.Response{OK: true}

	case "start":
		m.handleIPCStartStopRestart(msg, "start")

	case "stop":
		m.handleIPCStartStopRestart(msg, "stop")

	case "restart":
		m.handleIPCStartStopRestart(msg, "restart")

	default:
		msg.ResponseCh <- ipc.Response{
			OK:    false,
			Error: fmt.Sprintf("Unknown action: %s", msg.Request.Action),
		}
	}
}

func (m *Model) handleIPCAddApp(msg ipc.IPCRequestMsg) {
	if msg.Request.App == nil {
		msg.ResponseCh <- ipc.Response{OK: false, Error: "Missing app field"}
		return
	}

	var entry struct {
		Name    string `json:"name"`
		Dir     string `json:"dir"`
		Command string `json:"command"`
		Ports   []int  `json:"ports"`
	}
	if err := json.Unmarshal(msg.Request.App, &entry); err != nil {
		msg.ResponseCh <- ipc.Response{OK: false, Error: "Invalid app data"}
		return
	}

	// Resolve relative dir from cwd
	cwd := msg.Request.Cwd
	if cwd == "" {
		cwd = m.projectRoot
	}
	if entry.Dir != "" && !filepath.IsAbs(entry.Dir) {
		entry.Dir = filepath.Join(cwd, entry.Dir)
	}

	// Make dir relative to project root
	relDir, err := filepath.Rel(m.projectRoot, entry.Dir)
	if err != nil || strings.HasPrefix(relDir, "..") {
		msg.ResponseCh <- ipc.Response{OK: false, Error: "Directory is outside the project root"}
		return
	}
	if relDir == "" {
		relDir = "."
	}
	entry.Dir = relDir

	// Check for duplicates by name and directory (B6)
	for _, a := range m.apps {
		if a.Name == entry.Name {
			msg.ResponseCh <- ipc.Response{OK: false, Error: fmt.Sprintf("App %q already exists", entry.Name)}
			return
		}
		if a.Dir == entry.Dir {
			msg.ResponseCh <- ipc.Response{OK: false, Error: fmt.Sprintf("Directory %q is already registered as %q", entry.Dir, a.Name)}
			return
		}
	}

	app := config.App{
		Name:    entry.Name,
		Dir:     entry.Dir,
		Command: entry.Command,
		Ports:   entry.Ports,
	}
	if err := app.Validate(); err != nil {
		msg.ResponseCh <- ipc.Response{OK: false, Error: fmt.Sprintf("Invalid app: %s", err)}
		return
	}

	m.apps = append(m.apps, app)
	m.saveConfig()
	m.systemLog(fmt.Sprintf("App \"%s\" added via IPC (dir: %s)", entry.Name, entry.Dir))

	// Optionally auto-start
	if msg.Request.AutoStart {
		go m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
	}

	msg.ResponseCh <- ipc.Response{
		OK:      true,
		Name:    entry.Name,
		Message: fmt.Sprintf("Added \"%s\" to devctl", entry.Name),
	}
}

func (m *Model) handleIPCStartStopRestart(msg ipc.IPCRequestMsg, action string) {
	target := msg.Request.Target
	if target == "" {
		msg.ResponseCh <- ipc.Response{OK: false, Error: "Missing target (app name or \"all\")"}
		return
	}

	if target == "all" {
		var results []string
		for _, app := range m.apps {
			result := m.ipcAppAction(app, action)
			results = append(results, result)
		}
		msg.ResponseCh <- ipc.Response{
			OK:      true,
			Message: strings.Join(results, "; "),
		}
		return
	}

	app := m.findApp(target)
	if app == nil {
		msg.ResponseCh <- ipc.Response{OK: false, Error: fmt.Sprintf("App %q not found", target)}
		return
	}

	result := m.ipcAppAction(*app, action)
	msg.ResponseCh <- ipc.Response{OK: true, Name: app.Name, Message: result}
}

func (m *Model) ipcAppAction(app config.App, action string) string {
	switch action {
	case "start":
		status := m.procManager.GetStatus(app.Name)
		if status == process.StatusRunning {
			return fmt.Sprintf("%s is already running", app.Name)
		}
		// Check for port conflicts and auto-resolve
		cmd := app.Command
		for _, port := range app.Ports {
			if !process.IsPortFree(port) {
				altPort := process.SuggestAlternativePort(port)
				if altPort > 0 {
					m.systemLog(fmt.Sprintf("[ipc] Port %d in use, using %d for %s", port, altPort, app.Name))
					cmd = fmt.Sprintf("PORT=%d %s", altPort, cmd)
				}
			}
		}
		go m.procManager.Start(app.Name, cmd, app.Dir, m.appEnv(app.Env, app.VaultEnv))
		return fmt.Sprintf("%s starting", app.Name)
	case "stop":
		status := m.procManager.GetStatus(app.Name)
		if status != process.StatusRunning {
			return fmt.Sprintf("%s is not running", app.Name)
		}
		m.procManager.Stop(app.Name)
		return fmt.Sprintf("%s stopping", app.Name)
	case "restart":
		go m.procManager.Restart(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
		return fmt.Sprintf("%s restarting", app.Name)
	default:
		return fmt.Sprintf("unknown action %s", action)
	}
}
