package tui

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/process"
)

func (m *Model) processEvent(evt process.ProcessEvent) tea.Cmd {
	if evt.Type == process.EventOutput || evt.Type == process.EventStderrOutput {
		bufName := m.getSelectedBufName()
		if bufName == evt.AppName {
			logBuf := m.procManager.GetLogBuffer(bufName)
			if logBuf.Follow {
				viewHeight := m.logViewHeight()
				logBuf.SnapToBottom(viewHeight)
			}
		}
	}

	// Update PID tracking file on process state changes
	if evt.Type == process.EventStarted || evt.Type == process.EventStopped || evt.Type == process.EventCrashed {
		m.updatePIDFile()
	}

	// Register/unregister health checks
	if evt.Type == process.EventStarted {
		if app := m.findApp(evt.AppName); app != nil && app.HealthCheck != nil && app.HealthCheck.URL != "" {
			m.healthChecker.Register(app.Name, app.HealthCheck.URL, app.HealthCheck.Interval)
		}
	}
	if evt.Type == process.EventStopped || evt.Type == process.EventCrashed {
		m.healthChecker.Unregister(evt.AppName)
	}

	// Register/unregister resource monitor
	if evt.Type == process.EventStarted {
		if app := m.findApp(evt.AppName); app != nil {
			cfg := process.ThresholdConfig{}
			if app.ResourceLimits != nil {
				cfg.MaxCPUPercent = app.ResourceLimits.MaxCPU
				cfg.MaxMemoryMB = app.ResourceLimits.MaxMemoryMB
			}
			m.resourceMonitor.Register(app.Name, cfg)
		}
	}
	if evt.Type == process.EventStopped || evt.Type == process.EventCrashed {
		m.resourceMonitor.Unregister(evt.AppName)
	}

	// Register/unregister file watcher
	if evt.Type == process.EventStarted {
		// Clear restart-in-flight flag (file watch triggered restart is complete)
		m.fileWatchManager.SetRestartInFlight(evt.AppName, false)
		if app := m.findApp(evt.AppName); app != nil && app.Watch != nil {
			absDir := app.Dir
			if !filepath.IsAbs(absDir) {
				absDir = filepath.Join(m.projectRoot, absDir)
			}
			if err := m.fileWatchManager.Register(app.Name, absDir, app.Watch); err != nil {
				m.systemLog(fmt.Sprintf("[watch] Failed to watch %s: %v", app.Name, err))
			}
		}
	}
	if evt.Type == process.EventStopped || evt.Type == process.EventCrashed {
		m.fileWatchManager.Unregister(evt.AppName)
	}

	if evt.Type == process.EventErrorDetected {
		if m.errorStream == nil {
			m.notification = "Error detected! [x] view"
			m.notificationEnd = time.Now().Add(notifyDuration)
		}
		// If error stream is open and following, scroll to show new errors
		if m.errorStream != nil && m.errorStream.follow {
			entries := m.buildErrorEntries()
			if len(entries) > 0 {
				m.errorStream.cursor = len(entries) - 1
			}
		}
		return tea.Tick(notifyDuration, func(time.Time) tea.Msg { return clearNotificationMsg{} })
	}

	// Desktop notification and auto-restart on crash
	if evt.Type == process.EventCrashed && !m.quitting {
		// Send desktop notification if enabled for this app
		if app := m.findApp(evt.AppName); app != nil && app.Notifications != nil && *app.Notifications {
			go process.SendNotification("devctl", fmt.Sprintf("%s crashed (code %d)", evt.AppName, evt.Code))
		}

		return m.maybeAutoRestart(evt.AppName)
	}

	return nil
}

func (m Model) listenForProcessEvents() tea.Cmd {
	return func() tea.Msg {
		first := <-m.procManager.Events()
		batch := []process.ProcessEvent{first}
		for {
			select {
			case evt := <-m.procManager.Events():
				batch = append(batch, evt)
				if len(batch) >= 256 {
					return processEventBatchMsg(batch)
				}
			default:
				return processEventBatchMsg(batch)
			}
		}
	}
}

func (m Model) listenForResourceAlerts() tea.Cmd {
	ch := m.resourceMonitor.Alerts()
	return func() tea.Msg {
		alert := <-ch
		return resourceAlertMsg(alert)
	}
}

func (m Model) listenForFileWatchEvents() tea.Cmd {
	ch := m.fileWatchManager.Events()
	return func() tea.Msg {
		evt := <-ch
		return fileWatchRestartMsg{
			appName:  evt.AppName,
			filePath: evt.FilePath,
		}
	}
}

func (m Model) listenForConfigChange() tea.Cmd {
	if m.configWatcher == nil {
		return nil
	}
	ch := m.configWatcher.Changes()
	return func() tea.Msg {
		<-ch
		return configChangedMsg{}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
