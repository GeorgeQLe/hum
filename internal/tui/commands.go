package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/process"
)

func (m *Model) showStatus(args string) {
	list := m.apps
	if args != "" {
		list = nil
		for _, a := range m.apps {
			if a.Name == args {
				list = append(list, a)
			}
		}
		if len(list) == 0 {
			m.systemLog(fmt.Sprintf("Unknown app: %s", args))
			return
		}
	}
	if len(list) == 0 {
		m.systemLog("No apps configured.")
		return
	}

	m.systemLog(fmt.Sprintf("%-20s %-10s %-8s %-10s %-10s %s", "NAME", "STATUS", "PID", "UPTIME", "CPU/MEM", "PORTS"))
	for _, app := range list {
		status := m.procManager.GetStatus(app.Name)
		pid := "-"
		uptime := "-"
		resources := "-"
		if status == process.StatusRunning {
			if p := m.procManager.PID(app.Name); p != 0 {
				pid = fmt.Sprintf("%d", p)
				// Prefer monitor stats over one-shot ps
				if latest := m.resourceMonitor.GetLatest(app.Name); latest != nil {
					resources = fmt.Sprintf("%.1f%% %s", latest.CPUPercent, process.FormatMemory(latest.MemoryRSS))
				} else if usage, err := process.GetResourceUsage(p); err == nil {
					resources = fmt.Sprintf("%.1f%% %s", usage.CPUPercent, process.FormatMemory(usage.MemoryRSS))
				}
			}
			uptime = formatUptime(m.procManager.Uptime(app.Name))
		}
		ports := ""
		for i, p := range app.Ports {
			if i > 0 {
				ports += ", "
			}
			ports += fmt.Sprintf("%d", p)
		}
		m.systemLog(fmt.Sprintf("%-20s %-10s %-8s %-10s %-10s %s", app.Name, string(status), pid, uptime, resources, ports))
	}
}

func (m *Model) showList() {
	if len(m.apps) == 0 {
		m.systemLog("No apps configured.")
		return
	}
	for _, app := range m.apps {
		m.systemLog(app.Name)
		m.systemLog(fmt.Sprintf("  dir:     %s", app.Dir))
		m.systemLog(fmt.Sprintf("  command: %s", app.Command))
		ports := ""
		for i, p := range app.Ports {
			if i > 0 {
				ports += ", "
			}
			ports += fmt.Sprintf("%d", p)
		}
		m.systemLog(fmt.Sprintf("  ports:   %s", ports))
		if app.Group != "" {
			m.systemLog(fmt.Sprintf("  group:   %s", app.Group))
		}
		if len(app.DependsOn) > 0 {
			m.systemLog(fmt.Sprintf("  depends: %s", strings.Join(app.DependsOn, ", ")))
		}
	}
}

func (m *Model) handleAutoRestart(args string) {
	if args == "" {
		// Show current auto-restart status for all apps
		m.systemLog("Auto-Restart Status")
		for _, app := range m.apps {
			entry := m.procManager.GetEntry(app.Name)
			configEnabled := app.AutoRestart != nil && *app.AutoRestart
			runtimeDisabled := false
			restartCount := 0
			if entry != nil {
				runtimeDisabled, restartCount = entry.GetAutoRestartState()
			}
			var statusText string
			if configEnabled {
				if runtimeDisabled {
					statusText = "disabled (runtime)"
				} else {
					statusText = "enabled"
				}
			} else {
				statusText = "disabled (config)"
			}
			restartInfo := ""
			if restartCount > 0 {
				restartInfo = fmt.Sprintf(" [%d restarts]", restartCount)
			}
			m.systemLog(fmt.Sprintf("  %s: %s%s", app.Name, statusText, restartInfo))
		}
		m.systemLog("")
		m.systemLog("Usage: autorestart <name> [on|off]")
		return
	}

	parts := strings.Fields(args)
	name := parts[0]
	action := ""
	if len(parts) > 1 {
		action = strings.ToLower(parts[1])
	}

	app := m.findApp(name)
	if app == nil {
		m.systemLog(fmt.Sprintf("Unknown app: %s", name))
		return
	}

	entry := m.procManager.GetEntry(name)

	if action == "" {
		// Toggle
		if entry != nil {
			nowDisabled := entry.ToggleAutoRestart()
			status := "enabled"
			if nowDisabled {
				status = "disabled"
			}
			m.systemLog(fmt.Sprintf("Auto-restart for %s: %s (runtime)", name, status))
		} else {
			configEnabled := app.AutoRestart != nil && *app.AutoRestart
			status := "disabled"
			if configEnabled {
				status = "enabled"
			}
			m.systemLog(fmt.Sprintf("%s has not been started yet. Auto-restart config: %s", name, status))
		}
		return
	}

	if action == "on" {
		if entry != nil {
			entry.EnableAutoRestart()
		}
		m.systemLog(fmt.Sprintf("Auto-restart for %s: enabled (runtime)", name))
	} else if action == "off" {
		if entry != nil {
			entry.DisableAutoRestart()
		}
		m.systemLog(fmt.Sprintf("Auto-restart for %s: disabled (runtime)", name))
	} else {
		m.systemLog(fmt.Sprintf("Invalid action: %s. Use 'on' or 'off'.", action))
	}
}

func (m *Model) handleClearErrors(args string) {
	if args == "" {
		selectedName := m.getSelectedBufName()
		if selectedName == systemName {
			m.systemLog("No app selected. Use 'clear-errors <name>' or 'clear-errors all'")
			return
		}
		m.procManager.ClearErrors(selectedName)
		m.systemLog("Errors cleared for " + selectedName)
		return
	}
	if args == "all" {
		m.procManager.ClearAllErrors()
		m.systemLog("All errors cleared")
		return
	}
	if m.findApp(args) == nil {
		m.systemLog(fmt.Sprintf("Unknown app: %s", args))
		return
	}
	m.procManager.ClearErrors(args)
	m.systemLog("Errors cleared for " + args)
}

func (m *Model) handleExport(args string) {
	name := args
	if name == "" {
		// Use currently selected app
		name = m.getSelectedBufName()
		if name == systemName {
			m.systemLog("Usage: export <name> [file]")
			return
		}
	}

	// Parse optional filename
	parts := strings.Fields(name)
	appName := parts[0]
	filename := ""
	if len(parts) > 1 {
		filename = parts[1]
	}

	if m.findApp(appName) == nil && appName != systemName {
		m.systemLog(fmt.Sprintf("Unknown app: %s", appName))
		return
	}

	buf := m.procManager.GetLogBuffer(appName)
	lines, _, _ := buf.Snapshot()
	if len(lines) == 0 {
		m.systemLog(fmt.Sprintf("No log output for %s", appName))
		return
	}

	if filename == "" {
		filename = fmt.Sprintf("%s-%s.log", appName, time.Now().Format("20060102-150405"))
	}

	// Strip ANSI codes from output
	var out strings.Builder
	for _, line := range lines {
		out.WriteString(process.StripAnsi(line.Text))
		out.WriteString("\n")
	}

	if err := os.WriteFile(filename, []byte(out.String()), 0644); err != nil {
		m.systemLog(fmt.Sprintf("Error writing log file: %s", err))
		return
	}
	m.systemLog(fmt.Sprintf("Exported %d lines to %s", len(lines), filename))
}

func (m *Model) handlePin(args string, pin bool) {
	if args == "" {
		if pin {
			m.systemLog("Usage: pin <name>")
		} else {
			m.systemLog("Usage: unpin <name>")
		}
		return
	}

	app := m.findApp(args)
	if app == nil {
		m.systemLog(fmt.Sprintf("Unknown app: %s", args))
		return
	}

	app.Pinned = &pin
	m.saveConfig()

	if pin {
		m.systemLog(fmt.Sprintf("Pinned %s", args))
	} else {
		m.systemLog(fmt.Sprintf("Unpinned %s", args))
	}
}

func (m Model) handleRun(args string) tea.Cmd {
	parts := strings.Fields(args)
	if len(parts) < 1 {
		m.systemLog("Usage: run <name> [command-type]")
		return nil
	}

	appName := parts[0]
	cmdType := "dev"
	if len(parts) >= 2 {
		cmdType = parts[1]
	}

	app := m.findApp(appName)
	if app == nil {
		m.systemLog(fmt.Sprintf("Unknown app: %s", appName))
		return nil
	}

	// Look up command: try Commands map first, fall back to Command field for "dev"
	var cmd string
	if len(app.Commands) > 0 {
		c, ok := app.Commands[cmdType]
		if !ok {
			avail := make([]string, 0, len(app.Commands))
			for k := range app.Commands {
				avail = append(avail, k)
			}
			m.systemLog(fmt.Sprintf("Unknown command type %q for %s. Available: %s", cmdType, appName, strings.Join(avail, ", ")))
			return nil
		}
		cmd = c
	} else {
		// No Commands map — use Command field only for default "dev" type
		if cmdType != "dev" {
			m.systemLog(fmt.Sprintf("No custom commands defined for %s. Only default 'dev' is available.", appName))
			return nil
		}
		cmd = app.Command
	}

	// Snapshot before closure
	appDir := app.Dir
	appEnv := m.appEnv(app.Env, app.VaultEnv)
	pm := m.procManager
	return func() tea.Msg {
		pm.Restart(appName, cmd, appDir, appEnv)
		return commandDoneMsg{}
	}
}

// expandGroupTarget expands @group to a list of app names, or returns the target as-is.
func (m *Model) expandGroupTarget(target string) []string {
	if !strings.HasPrefix(target, "@") {
		return []string{target}
	}
	groupName := target[1:]
	var names []string
	for _, app := range m.apps {
		if app.Group == groupName {
			names = append(names, app.Name)
		}
	}
	if len(names) == 0 {
		m.systemLog(fmt.Sprintf("No apps in group @%s", groupName))
	}
	return names
}

func (m *Model) showHelp() {
	m.systemLog("devctl — Multi-App Dev Server Manager")
	m.systemLog("")
	m.systemLog("  start <name|all>        Start an app (or all)")
	m.systemLog("  stop <name|all>         Stop an app (or all)")
	m.systemLog("  restart <name|all>      Restart an app (or all)")
	m.systemLog("  status [name]           Show app status table")
	m.systemLog("  ports                   Check port availability")
	m.systemLog("  scan                    Auto-detect apps (batch select)")
	m.systemLog("  add                     Add a new app interactively")
	m.systemLog("  remove <name>           Remove an app from config")
	m.systemLog("  reload                  Reload config from apps.json")
	m.systemLog("  autorestart [name]      View/toggle auto-restart")
	m.systemLog("  clear-errors [name|all] Clear detected errors")
	m.systemLog("  export <name> [file]    Export app logs to file")
	m.systemLog("  pin/unpin <name>        Pin/unpin app to top of sidebar")
	m.systemLog("  run <name> <type>       Run a custom command type")
	m.systemLog("  top                     Live resource dashboard")
	m.systemLog("  watch [name] [on|off]   View/toggle file watching")
	m.systemLog("  list                    List configured apps")
	m.systemLog("  help                    Show this help")
	m.systemLog("  quit                    Stop all and exit")
	m.systemLog("")
	m.systemLog("Flags:")
	m.systemLog("  --start-all    Start all apps on launch")
	m.systemLog("  --restore      Restore previous session")
	m.systemLog("")
	m.systemLog("Remote commands (from another terminal):")
	m.systemLog("  devctl add <dir>           Add app from directory")
	m.systemLog("  devctl add <dir> --start   Add and start immediately")
	m.systemLog("  devctl status              Show running instance status")
	m.systemLog("  devctl stats               Show resource statistics")
	m.systemLog("  devctl stats --watch       Live resource monitoring")
	m.systemLog("  devctl stats --json        JSON output for scripting")
	m.systemLog("  devctl ping                Check if devctl is running")
	m.systemLog("")
	m.systemLog("Tab: toggle sidebar/command  up/down/j/k: navigate  PgUp/PgDn: scroll")
	m.systemLog("Ctrl+J/K: scroll line  Ctrl+B: toggle sidebar")
	m.systemLog("/: search  t: timestamps  x: error stream  e/E: copy errors  s/S/r: start/stop/restart  ^C: quit")
}

func (m *Model) handleWatch(args string) {
	if args == "" {
		// Show watch status for all apps
		m.systemLog("File watch status:")
		for _, app := range m.apps {
			status := "not configured"
			if app.Watch != nil {
				if m.fileWatchManager.HasWatch(app.Name) {
					if m.fileWatchManager.IsEnabled(app.Name) {
						status = "active"
					} else {
						status = "paused"
					}
				} else {
					status = "configured (not running)"
				}
			}
			m.systemLog(fmt.Sprintf("  %s: %s", app.Name, status))
		}
		return
	}

	parts := strings.Fields(args)
	name := parts[0]
	app := m.findApp(name)
	if app == nil {
		m.systemLog(fmt.Sprintf("Unknown app: %s", name))
		return
	}

	if app.Watch == nil {
		m.systemLog(fmt.Sprintf("%s has no watch config. Add \"watch\": {} to apps.json.", name))
		return
	}

	if len(parts) >= 2 {
		switch parts[1] {
		case "on":
			m.fileWatchManager.SetEnabled(name, true)
			m.systemLog(fmt.Sprintf("File watching enabled for %s", name))
		case "off":
			m.fileWatchManager.SetEnabled(name, false)
			m.systemLog(fmt.Sprintf("File watching paused for %s", name))
		default:
			m.systemLog("Usage: watch <name> [on|off]")
		}
		return
	}

	// Toggle
	newState := m.fileWatchManager.Toggle(name)
	if newState {
		m.systemLog(fmt.Sprintf("File watching enabled for %s", name))
	} else {
		m.systemLog(fmt.Sprintf("File watching paused for %s", name))
	}
}

func (m *Model) maybeAutoRestart(appName string) tea.Cmd {
	app := m.findApp(appName)
	if app == nil {
		return nil
	}

	// Check config
	if app.AutoRestart == nil || !*app.AutoRestart {
		return nil
	}

	entry := m.procManager.GetEntry(appName)
	if entry == nil {
		return nil
	}

	maxRestarts := 5
	if app.MaxRestarts != nil {
		maxRestarts = *app.MaxRestarts
	}
	restartDelay := 3000
	if app.RestartDelay != nil {
		restartDelay = *app.RestartDelay
	}

	canRestart, count := entry.TryAutoRestart(maxRestarts)
	if !canRestart {
		if count >= maxRestarts {
			m.systemLog(fmt.Sprintf("Auto-restart limit reached for %s (%d attempts). Use 'start %s' to restart manually.", appName, maxRestarts, appName))
		}
		return nil
	}

	m.systemLog(fmt.Sprintf("Auto-restarting %s in %dms (attempt %d/%d)...", appName, restartDelay, count, maxRestarts))

	delay := time.Duration(restartDelay) * time.Millisecond
	name := appName
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return autoRestartMsg{appName: name}
	})
}

func (m *Model) handlePortConflict(msg portConflictMsg) {
	app := m.findApp(msg.appName)
	if app == nil {
		return
	}

	// Handle the first conflict with a question
	conflict := msg.conflicts[0]
	if conflict.owner == nil {
		m.askQuestion(fmt.Sprintf("Port %d in use. Start %s anyway? (y/N): ", conflict.port, msg.appName), func(answer string) {
			if strings.ToLower(answer) == "y" {
				m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			} else {
				logBuf := m.procManager.GetLogBuffer(app.Name)
				logBuf.Append("Start cancelled.", false)
			}
		})
		return
	}

	devctlApp := m.procManager.FindDevctlOwner(conflict.owner.PID)
	if devctlApp != "" {
		m.askQuestion(fmt.Sprintf("Port %d used by %s. [r]estart/[a]lt port/[s]tart anyway/[c]ancel: ", conflict.port, devctlApp), func(answer string) {
			switch strings.ToLower(answer) {
			case "r":
				// Look up the conflicting app's command/dir
				conflictApp := m.findApp(devctlApp)
				if conflictApp != nil {
					m.procManager.Restart(devctlApp, conflictApp.Command, conflictApp.Dir, m.appEnv(conflictApp.Env, conflictApp.VaultEnv))
				}
				if !process.WaitForPortFree(conflict.port, 2*time.Second) {
					m.systemLog(fmt.Sprintf("Port %d still in use after restart — start aborted.", conflict.port))
					return
				}
				m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			case "a":
				altPort := process.SuggestAlternativePort(conflict.port)
				if altPort > 0 {
					m.systemLog(fmt.Sprintf("Using alternative port %d for %s", altPort, app.Name))
					modifiedCmd := fmt.Sprintf("PORT=%d %s", altPort, app.Command)
					m.procManager.Start(app.Name, modifiedCmd, app.Dir, m.appEnv(app.Env, app.VaultEnv))
				} else {
					m.systemLog("No alternative port found.")
				}
			case "s":
				m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			case "c", "":
				logBuf := m.procManager.GetLogBuffer(app.Name)
				logBuf.Append("Start cancelled.", false)
			default:
				m.systemLog(fmt.Sprintf("Invalid choice: %q. Use [r]estart/[a]lt/[s]tart/[c]ancel.", answer))
			}
		})
	} else {
		m.askQuestion(fmt.Sprintf("Port %d used by %s (PID %d). [k]ill/[a]lt port/[s]tart anyway/[c]ancel: ", conflict.port, conflict.owner.Command, conflict.owner.PID), func(answer string) {
			switch strings.ToLower(answer) {
			case "k":
				process.KillExternalProcess(conflict.owner.PID)
				if !process.WaitForPortFree(conflict.port, 2*time.Second) {
					m.systemLog(fmt.Sprintf("Port %d still in use after killing PID %d — start aborted.", conflict.port, conflict.owner.PID))
					return
				}
				m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			case "a":
				altPort := process.SuggestAlternativePort(conflict.port)
				if altPort > 0 {
					m.systemLog(fmt.Sprintf("Using alternative port %d for %s", altPort, app.Name))
					modifiedCmd := fmt.Sprintf("PORT=%d %s", altPort, app.Command)
					m.procManager.Start(app.Name, modifiedCmd, app.Dir, m.appEnv(app.Env, app.VaultEnv))
				} else {
					m.systemLog("No alternative port found.")
				}
			case "s":
				m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			case "c", "":
				logBuf := m.procManager.GetLogBuffer(app.Name)
				logBuf.Append("Start cancelled.", false)
			default:
				m.systemLog(fmt.Sprintf("Invalid choice: %q. Use [k]ill/[a]lt/[s]tart/[c]ancel.", answer))
			}
		})
	}
}

func (m *Model) startAddWizard() {
	m.askQuestion("App name: ", func(name string) {
		if name == "" {
			m.systemLog("Cancelled.")
			return
		}
		if m.findApp(name) != nil {
			m.systemLog(fmt.Sprintf("App \"%s\" already exists.", name))
			return
		}
		m.askQuestion("Directory (relative to project root): ", func(dir string) {
			if dir == "" {
				m.systemLog("Cancelled.")
				return
			}
			m.askQuestion("Command: ", func(command string) {
				if command == "" {
					m.systemLog("Cancelled.")
					return
				}
				m.askQuestion("Ports (comma-separated): ", func(portsStr string) {
					if portsStr == "" {
						m.systemLog("Cancelled.")
						return
					}
					var ports []int
					for _, s := range strings.Split(portsStr, ",") {
						s = strings.TrimSpace(s)
						if s == "" {
							continue
						}
						var p int
						if _, err := fmt.Sscanf(s, "%d", &p); err == nil && p > 0 && p < 65536 {
							ports = append(ports, p)
						}
					}
					if len(ports) == 0 {
						m.systemLog("No valid ports provided.")
						return
					}
					app := config.App{
						Name:    name,
						Dir:     dir,
						Command: command,
						Ports:   ports,
					}
					if err := app.Validate(); err != nil {
						m.systemLog(fmt.Sprintf("Invalid entry: %s", err))
						return
					}
					m.apps = append(m.apps, app)
					m.saveConfig()
					m.systemLog(fmt.Sprintf("Added %s.", name))
				})
			})
		})
	})
}

func (m Model) handleRemove(args string) (tea.Model, tea.Cmd) {
	if args == "" {
		m.systemLog("Usage: remove <name>")
		return m, nil
	}
	app := m.findApp(args)
	if app == nil {
		m.systemLog(fmt.Sprintf("Unknown app: %s", args))
		return m, nil
	}

	name := args
	status := m.procManager.GetStatus(name)
	if status == process.StatusRunning {
		m.askQuestion(fmt.Sprintf("%s is running. Stop it first? (y/N): ", name), func(answer string) {
			if strings.ToLower(answer) != "y" {
				m.systemLog("Remove cancelled.")
				return
			}
			m.procManager.Stop(name)
			m.removeApp(name)
		})
		return m, nil
	}

	m.removeApp(name)
	return m, nil
}

func (m *Model) removeApp(name string) {
	newApps := make([]config.App, 0, len(m.apps))
	for _, a := range m.apps {
		if a.Name != name {
			newApps = append(newApps, a)
		}
	}
	m.apps = newApps
	m.saveConfig()
	// Clean up stale entries from the process manager
	m.procManager.RemoveEntries(name)
	m.systemLog(fmt.Sprintf("Removed %s.", name))
	if m.selectedIdx > len(m.apps) {
		m.selectedIdx = len(m.apps)
	}
}

func (m Model) checkPortsAsync() tea.Cmd {
	apps := make([]config.App, len(m.apps))
	copy(apps, m.apps)
	return func() tea.Msg {
		var results []portCheckResult
		for _, app := range apps {
			for _, p := range app.Ports {
				free := process.IsPortFree(p)
				var owner *process.PortOwnerInfo
				if !free {
					owner = process.GetPortOwnerInfo(p)
				}
				results = append(results, portCheckResult{
					port:    p,
					free:    free,
					appName: app.Name,
					owner:   owner,
				})
			}
		}
		return portCheckResultMsg{results: results}
	}
}

func (m Model) scanAsync() tea.Cmd {
	projectRoot := m.projectRoot
	apps := make([]config.App, len(m.apps))
	copy(apps, m.apps)
	return func() tea.Msg {
		candidates := config.DetectApps(projectRoot, apps)
		return scanResultMsg{candidates: candidates}
	}
}

func (m *Model) showPortResults(results []portCheckResult) {
	if len(results) == 0 {
		m.systemLog("No apps configured.")
		return
	}
	m.systemLog(fmt.Sprintf("%-8s %-10s %-20s %s", "PORT", "STATUS", "APP", "OWNER"))
	for _, r := range results {
		status := "free"
		if !r.free {
			status = "in use"
		}
		ownerStr := ""
		if !r.free && r.owner != nil {
			devctlApp := m.procManager.FindDevctlOwner(r.owner.PID)
			if devctlApp != "" {
				ownerStr = "devctl:" + devctlApp
			} else {
				ownerStr = fmt.Sprintf("%s (PID %d)", r.owner.Command, r.owner.PID)
			}
		} else if !r.free {
			ownerStr = "unknown"
		}
		m.systemLog(fmt.Sprintf("%-8d %-10s %-20s %s", r.port, status, r.appName, ownerStr))
	}
}

func (m Model) copyLastError() (tea.Model, tea.Cmd) {
	app := m.getSelectedApp()
	if app == nil {
		return m, nil
	}
	errBuf := m.procManager.GetErrorBuffer(app.Name)
	text := errBuf.LastErrorText()
	if text == "" {
		m.systemLog("No errors captured for " + app.Name)
		return m, nil
	}
	if err := process.CopyToClipboard(text); err != nil {
		m.systemLog(fmt.Sprintf("Clipboard error: %s", err))
	} else {
		m.systemLog("Last error copied to clipboard")
	}
	return m, nil
}

func (m Model) copyAllErrors() (tea.Model, tea.Cmd) {
	app := m.getSelectedApp()
	if app == nil {
		return m, nil
	}
	errBuf := m.procManager.GetErrorBuffer(app.Name)
	text := errBuf.AllErrorsText()
	if text == "" {
		m.systemLog("No errors captured for " + app.Name)
		return m, nil
	}
	if err := process.CopyToClipboard(text); err != nil {
		m.systemLog(fmt.Sprintf("Clipboard error: %s", err))
	} else {
		count := errBuf.Count()
		m.systemLog(fmt.Sprintf("All %d error(s) copied to clipboard", count))
	}
	return m, nil
}
