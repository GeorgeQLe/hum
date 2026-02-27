package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/hum/internal/api"
	"github.com/georgele/hum/internal/config"
	"github.com/georgele/hum/internal/process"
	"github.com/georgele/hum/internal/state"
)

func (m Model) executeAsync(action, target string) tea.Cmd {
	pm := m.procManager
	apps := make([]config.App, len(m.apps))
	copy(apps, m.apps)
	findApp := func(name string) *config.App {
		for i := range apps {
			if apps[i].Name == name {
				return &apps[i]
			}
		}
		return nil
	}

	// Expand @group targets
	if strings.HasPrefix(target, "@") {
		names := m.expandGroupTarget(target)
		if len(names) == 0 {
			return nil
		}
		return func() tea.Msg {
			for _, name := range names {
				app := findApp(name)
				if app == nil {
					continue
				}
				switch action {
				case "start":
					pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
				case "stop":
					pm.Stop(app.Name)
				case "restart":
					pm.Restart(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
				}
			}
			return commandDoneMsg{}
		}
	}

	return func() tea.Msg {
		switch action {
		case "start":
			if target == "all" {
				// Sort apps topologically for dependency-aware start order
				sorted, sortErr := config.TopologicalSort(apps)
				if sortErr != nil {
					pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Dependency warning: %s — starting in config order", sortErr), false)
					sorted = apps
				}
				// Pre-check ports for all apps, start conflict-free ones in order
				type conflictInfo struct {
					appName string
					port    int
					owner   *process.PortOwnerInfo
				}
				var conflicts []conflictInfo
				for _, app := range sorted {
					status := pm.GetStatus(app.Name)
					if status == process.StatusRunning {
						continue
					}
					hasConflict := false
					for _, p := range app.Ports {
						if !process.IsPortFree(p) {
							hasConflict = true
							owner := process.GetPortOwnerInfo(p)
							conflicts = append(conflicts, conflictInfo{
								appName: app.Name,
								port:    p,
								owner:   owner,
							})
							break
						}
					}
					if !hasConflict {
						pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
					}
				}
				// Return conflict messages for individual resolution (B2)
				if len(conflicts) > 0 {
					names := make([]string, len(conflicts))
					for i, c := range conflicts {
						names[i] = c.appName
						logBuf := pm.GetLogBuffer(c.appName)
						if c.owner != nil {
							logBuf.Append(fmt.Sprintf("Port %d in use by %s (PID %d)", c.port, c.owner.Command, c.owner.PID), false)
						} else {
							logBuf.Append(fmt.Sprintf("Port %d in use", c.port), false)
						}
					}
					pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Port conflicts: %s — use 'start <name>' to resolve individually", strings.Join(names, ", ")), false)
					// Return port conflict for first conflicting app to trigger interactive resolution
					if len(conflicts) > 0 {
						c := conflicts[0]
						return portConflictMsg{
							appName: c.appName,
							conflicts: []struct {
								port  int
								owner *process.PortOwnerInfo
							}{{c.port, c.owner}},
						}
					}
				}
			} else {
				app := findApp(target)
				if app == nil {
					pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Unknown app: %s", target), false)
				} else {
					// Auto-start dependencies first
					deps, depErr := config.DependencyOrder(apps, target)
					if depErr != nil {
						pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Warning: %v", depErr), false)
					}
					for _, depName := range deps {
						if pm.GetStatus(depName) != process.StatusRunning {
							depApp := findApp(depName)
							if depApp != nil {
								pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Starting dependency %s for %s", depName, target), false)
								pm.Start(depApp.Name, depApp.Command, depApp.Dir, pm.ResolveEnv(depApp.Env, depApp.VaultEnv))
							}
						}
					}
					// Check ports before starting
					var taken []struct {
						port  int
						owner *process.PortOwnerInfo
					}
					for _, p := range app.Ports {
						if !process.IsPortFree(p) {
							owner := process.GetPortOwnerInfo(p)
							taken = append(taken, struct {
								port  int
								owner *process.PortOwnerInfo
							}{p, owner})
						}
					}
					if len(taken) > 0 {
						return portConflictMsg{appName: app.Name, conflicts: taken}
					}
					pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
				}
			}
		case "stop":
			if target == "all" {
				pm.StopAll()
			} else {
				pm.Stop(target)
			}
		case "restart":
			if target == "all" {
				for _, app := range apps {
					pm.Stop(app.Name)
				}
				for _, app := range apps {
					for _, p := range app.Ports {
						process.WaitForPortFree(p, 3*time.Second)
					}
				}
				for _, app := range apps {
					var taken []struct {
						port  int
						owner *process.PortOwnerInfo
					}
					for _, p := range app.Ports {
						if !process.IsPortFree(p) {
							owner := process.GetPortOwnerInfo(p)
							taken = append(taken, struct {
								port  int
								owner *process.PortOwnerInfo
							}{p, owner})
						}
					}
					if len(taken) > 0 {
						return portConflictMsg{appName: app.Name, conflicts: taken}
					}
					pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
				}
			} else {
				app := findApp(target)
				if app == nil {
					pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Unknown app: %s", target), false)
				} else {
					pm.Stop(app.Name)
					for _, p := range app.Ports {
						process.WaitForPortFree(p, 3*time.Second)
					}
					var taken []struct {
						port  int
						owner *process.PortOwnerInfo
					}
					for _, p := range app.Ports {
						if !process.IsPortFree(p) {
							owner := process.GetPortOwnerInfo(p)
							taken = append(taken, struct {
								port  int
								owner *process.PortOwnerInfo
							}{p, owner})
						}
					}
					if len(taken) > 0 {
						return portConflictMsg{appName: app.Name, conflicts: taken}
					}
					pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
				}
			}
		}
		return commandDoneMsg{}
	}
}

func (m *Model) findApp(name string) *config.App {
	for i := range m.apps {
		if m.apps[i].Name == name {
			return &m.apps[i]
		}
	}
	return nil
}

func (m Model) startAllCmd() tea.Cmd {
	pm := m.procManager
	apps := make([]config.App, len(m.apps))
	copy(apps, m.apps)
	return func() tea.Msg {
		sorted, err := config.TopologicalSort(apps)
		if err != nil {
			sorted = apps
		}
		for _, app := range sorted {
			pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
		}
		return commandDoneMsg{}
	}
}

func (m Model) restoreSessionCmd() tea.Cmd {
	pm := m.procManager
	projectRoot := m.projectRoot
	apps := make([]config.App, len(m.apps))
	copy(apps, m.apps)
	return func() tea.Msg {
		saved := state.LoadSession(projectRoot)
		if len(saved) == 0 {
			pm.GetLogBuffer(systemName).Append("No previous session to restore.", false)
			return commandDoneMsg{}
		}

		appMap := make(map[string]config.App)
		for _, a := range apps {
			appMap[a.Name] = a
		}

		// Build a filtered list of apps to restore, then sort topologically
		var toRestore []config.App
		var missing []string
		for _, name := range saved {
			if app, ok := appMap[name]; ok {
				toRestore = append(toRestore, app)
			} else {
				missing = append(missing, name)
			}
		}

		// Sort by dependencies
		sorted, sortErr := config.TopologicalSort(toRestore)
		if sortErr != nil {
			pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Dependency warning: %s — restoring in saved order", sortErr), false)
			sorted = toRestore
		}

		restored := 0
		var skippedConflicts []string
		for _, app := range sorted {
			hasConflict := false
			for _, p := range app.Ports {
				if !process.IsPortFree(p) {
					hasConflict = true
					owner := process.GetPortOwnerInfo(p)
					logBuf := pm.GetLogBuffer(app.Name)
					if owner != nil {
						logBuf.Append(fmt.Sprintf("Port %d in use by %s (PID %d) — skipped during restore", p, owner.Command, owner.PID), false)
					} else {
						logBuf.Append(fmt.Sprintf("Port %d in use — skipped during restore", p), false)
					}
					break
				}
			}
			if hasConflict {
				skippedConflicts = append(skippedConflicts, app.Name)
				continue
			}
			pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
			restored++
		}
		if len(skippedConflicts) > 0 {
			pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Port conflicts during restore: %s — use 'start <name>' to resolve individually", strings.Join(skippedConflicts, ", ")), false)
		}

		state.ClearSession(projectRoot)
		pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Restored %d app(s) from previous session.", restored), false)
		if len(missing) > 0 {
			pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Warning: %d app(s) from previous session no longer in config: %s", len(missing), strings.Join(missing, ", ")), false)
		}
		return commandDoneMsg{}
	}
}

func (m Model) cleanupOrphansCmd() tea.Cmd {
	pm := m.procManager
	return func() tea.Msg {
		cleaned := api.CleanupOrphans()
		if len(cleaned) > 0 {
			pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Cleaned up %d orphaned process(es): %s", len(cleaned), strings.Join(cleaned, ", ")), false)
		}
		return commandDoneMsg{}
	}
}

func (m Model) autoStartCmd() tea.Cmd {
	pm := m.procManager
	apps := make([]config.App, len(m.apps))
	copy(apps, m.apps)
	return func() tea.Msg {
		var toStart []config.App
		for _, app := range apps {
			if app.AutoStart {
				toStart = append(toStart, app)
			}
		}
		if len(toStart) == 0 {
			return commandDoneMsg{}
		}

		// Sort by dependencies
		sorted, err := config.TopologicalSort(toStart)
		if err != nil {
			sorted = toStart
		}

		for _, app := range sorted {
			pm.Start(app.Name, app.Command, app.Dir, pm.ResolveEnv(app.Env, app.VaultEnv))
		}
		pm.GetLogBuffer(systemName).Append(fmt.Sprintf("Auto-started %d app(s).", len(sorted)), false)
		return commandDoneMsg{}
	}
}

// updatePIDFile writes current managed process PIDs to ~/.humrun/pids.json.
func (m *Model) updatePIDFile() {
	var entries []api.PIDEntry
	for _, app := range m.apps {
		pid := m.procManager.PID(app.Name)
		if pid > 0 {
			entries = append(entries, api.PIDEntry{
				AppName: app.Name,
				PID:     pid,
			})
		}
	}
	api.WritePIDFile(api.PIDFile{
		HumrunPID: os.Getpid(),
		Entries:   entries,
	})
}
