package tui

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/process"
	"github.com/georgele/devctl/internal/state"
)

func (m Model) handleQuit() (tea.Model, tea.Cmd) {
	m.quitting = true
	if m.healthChecker != nil {
		m.healthChecker.StopAll()
	}
	if m.resourceMonitor != nil {
		m.resourceMonitor.StopAll()
	}
	if m.configWatcher != nil {
		m.configWatcher.Stop()
	}
	if m.ipcServer != nil {
		m.ipcServer.Stop()
	}
	m.stopAPIServer()
	api.RemovePIDFile()
	if m.fileWatchManager != nil {
		m.fileWatchManager.StopAll()
	}
	// Capture which apps are running BEFORE stopping (so we know what to save)
	var running []string
	for _, app := range m.apps {
		if m.procManager.GetStatus(app.Name) == process.StatusRunning {
			running = append(running, app.Name)
		}
	}
	pm := m.procManager
	projectRoot := m.projectRoot
	return m, func() tea.Msg {
		done := make(chan struct{})
		go func() {
			pm.StopAll()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(quitTimeout):
		}
		// Save session state AFTER stopping completes (B10)
		state.SaveSession(projectRoot, running)
		return quitDoneMsg{}
	}
}

func (m Model) handleReload() (tea.Model, tea.Cmd) {
	m.systemLog("Reloading config from apps.json...")
	newApps, err := config.Load(m.projectRoot)
	if err != nil {
		m.systemLog(fmt.Sprintf("Error loading config: %s", err))
		return m, nil
	}

	// Detect changes
	oldMap := make(map[string]config.App)
	for _, a := range m.apps {
		oldMap[a.Name] = a
	}
	newMap := make(map[string]config.App)
	for _, a := range newApps {
		newMap[a.Name] = a
	}

	var added, removed, changed []string
	for name := range newMap {
		if _, ok := oldMap[name]; !ok {
			added = append(added, name)
		}
	}
	for name := range oldMap {
		if _, ok := newMap[name]; !ok {
			removed = append(removed, name)
		}
	}
	for name, newApp := range newMap {
		if oldApp, ok := oldMap[name]; ok {
			if config.HasChanged(oldApp, newApp) {
				changed = append(changed, name)
			}
		}
	}

	if len(added) > 0 {
		m.systemLog(fmt.Sprintf("Added: %s", strings.Join(added, ", ")))
	}
	if len(removed) > 0 {
		m.systemLog(fmt.Sprintf("Removed: %s", strings.Join(removed, ", ")))
		// Offer to stop removed apps that are still running (B4)
		for _, name := range removed {
			if m.procManager.GetStatus(name) == process.StatusRunning {
				n := name
				m.askQuestion(fmt.Sprintf("%s was removed from config but is still running. Stop it? (y/N): ", n), func(answer string) {
					if strings.ToLower(answer) == "y" {
						m.procManager.Stop(n)
						m.systemLog(fmt.Sprintf("Stopped %s.", n))
					}
				})
			}
		}
	}
	if len(changed) > 0 {
		m.systemLog(fmt.Sprintf("Changed: %s", strings.Join(changed, ", ")))
		// Show what changed for each app (B5)
		for _, name := range changed {
			oldApp := oldMap[name]
			newApp := newMap[name]
			if oldApp.Dir != newApp.Dir {
				m.systemLog(fmt.Sprintf("  %s: dir: %s -> %s", name, oldApp.Dir, newApp.Dir))
			}
			if oldApp.Command != newApp.Command {
				m.systemLog(fmt.Sprintf("  %s: command: %s -> %s", name, oldApp.Command, newApp.Command))
			}
			if fmt.Sprint(oldApp.Ports) != fmt.Sprint(newApp.Ports) {
				m.systemLog(fmt.Sprintf("  %s: ports: %v -> %v", name, oldApp.Ports, newApp.Ports))
			}
			if m.procManager.GetStatus(name) == process.StatusRunning {
				n := name
				newApp := newMap[n]
				m.askQuestion(fmt.Sprintf("%s is running with old config. Restart with new config? (y/N): ", n), func(answer string) {
					if strings.ToLower(answer) == "y" {
						m.procManager.Restart(n, newApp.Command, newApp.Dir, m.appEnv(newApp.Env, newApp.VaultEnv))
						m.systemLog(fmt.Sprintf("Restarted %s with new config.", n))
					}
				})
			}
		}
	}
	if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
		m.systemLog("No changes detected.")
	}

	// Re-register file watchers for running apps whose watch config changed
	for _, name := range changed {
		if m.procManager.GetStatus(name) == process.StatusRunning {
			oldApp := oldMap[name]
			newApp := newMap[name]
			if !reflect.DeepEqual(oldApp.Watch, newApp.Watch) {
				m.fileWatchManager.Unregister(name)
				if newApp.Watch != nil {
					absDir := newApp.Dir
					if !filepath.IsAbs(absDir) {
						absDir = filepath.Join(m.projectRoot, absDir)
					}
					if err := m.fileWatchManager.Register(name, absDir, newApp.Watch); err != nil {
						m.systemLog(fmt.Sprintf("[watch] Failed to re-register %s: %v", name, err))
					} else {
						m.systemLog(fmt.Sprintf("[watch] Re-registered watcher for %s", name))
					}
				}
			}
		}
	}

	m.apps = newApps
	if m.selectedIdx > len(m.apps) {
		m.selectedIdx = len(m.apps)
	}
	m.appsSnap.refresh(m.apps)
	m.systemLog("Config reloaded successfully.")
	return m, nil
}

func (m Model) handleDevReload() (tea.Model, tea.Cmd) {
	m.quitting = true
	// Save session state
	var running []string
	for _, app := range m.apps {
		if m.procManager.GetStatus(app.Name) == process.StatusRunning {
			running = append(running, app.Name)
		}
	}
	state.SaveSession(m.projectRoot, running)
	// Stop watchers and IPC, but NOT managed apps
	if m.healthChecker != nil {
		m.healthChecker.StopAll()
	}
	if m.resourceMonitor != nil {
		m.resourceMonitor.StopAll()
	}
	if m.configWatcher != nil {
		m.configWatcher.Stop()
	}
	if m.ipcServer != nil {
		m.ipcServer.Stop()
	}
	m.stopAPIServer()
	if m.fileWatchManager != nil {
		m.fileWatchManager.StopAll()
	}
	return m, tea.Quit
}
