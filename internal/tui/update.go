package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/ipc"
	"github.com/georgele/devctl/internal/process"
)

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.quitting {
			return m, nil
		}
		return m.handleKeypress(msg)

	case processEventMsg:
		cmds := []tea.Cmd{m.listenForProcessEvents()}
		if cmd := m.processEvent(process.ProcessEvent(msg)); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case processEventBatchMsg:
		var cmds []tea.Cmd
		for _, evt := range msg {
			if cmd := m.processEvent(evt); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		cmds = append(cmds, m.listenForProcessEvents())
		return m, tea.Batch(cmds...)

	case clearNotificationMsg:
		m.notification = ""
		return m, nil

	case autoRestartMsg:
		if m.quitting {
			return m, nil
		}
		app := m.findApp(msg.appName)
		if app == nil {
			return m, nil
		}
		// Check if still crashed before restarting
		if m.procManager.GetStatus(msg.appName) == process.StatusCrashed {
			return m, m.executeAsync("start", msg.appName)
		}
		return m, nil

	case portCheckResultMsg:
		m.showPortResults(msg.results)
		return m, nil

	case portConflictMsg:
		m.handlePortConflict(msg)
		return m, nil

	case scanResultMsg:
		if len(msg.candidates) == 0 {
			m.systemLog("No new apps detected.")
		} else {
			m.scanMode = newScanMode(msg.candidates)
		}
		return m, nil

	case configChangedMsg:
		m.systemLog("Config file changed, reloading...")
		newM, cmd := m.handleReload()
		mPtr := newM.(Model)
		return mPtr, tea.Batch(cmd, mPtr.listenForConfigChange())

	case ipcRequestMsg:
		m.handleIPCRequest(ipc.IPCRequestMsg(msg))
		return m, m.listenForIPCRequests()

	case resourceAlertMsg:
		alert := process.ThresholdAlert(msg)
		var alertText string
		if alert.Type == process.AlertCPU {
			alertText = fmt.Sprintf("CPU alert: %s at %.1f%% (limit: %.0f%%)", alert.AppName, alert.Value, alert.Threshold)
		} else {
			alertText = fmt.Sprintf("Memory alert: %s at %.0fMB (limit: %.0fMB)", alert.AppName, alert.Value, alert.Threshold)
		}
		m.systemLog(alertText)
		m.notification = alertText
		m.notificationEnd = time.Now().Add(notifyDuration)
		// Send desktop notification if enabled for this app
		if app := m.findApp(alert.AppName); app != nil && app.Notifications != nil && *app.Notifications {
			go process.SendNotification("devctl", alertText)
		}
		return m, tea.Batch(
			m.listenForResourceAlerts(),
			tea.Tick(notifyDuration, func(time.Time) tea.Msg { return clearNotificationMsg{} }),
		)

	case fileWatchRestartMsg:
		if m.quitting {
			return m, nil
		}
		app := m.findApp(msg.appName)
		if app == nil {
			return m, nil
		}
		if m.procManager.GetStatus(msg.appName) != process.StatusRunning {
			return m, nil
		}
		relPath, _ := filepath.Rel(m.projectRoot, msg.filePath)
		if relPath == "" {
			relPath = msg.filePath
		}
		logMsg := fmt.Sprintf("[watch] %s: %s changed, restarting...", msg.appName, relPath)
		m.systemLog(logMsg)
		buf := m.procManager.GetLogBuffer(msg.appName)
		buf.Append(logMsg, false)
		m.notification = fmt.Sprintf("%s restarting (file changed)", msg.appName)
		m.notificationEnd = time.Now().Add(3 * time.Second)
		m.fileWatchManager.SetRestartInFlight(msg.appName, true)
		restartCmd := m.executeAsync("restart", msg.appName)
		return m, tea.Batch(
			restartCmd,
			m.listenForFileWatchEvents(),
			tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearNotificationMsg{} }),
		)

	case approvalNotifyMsg:
		m.approvalMode = true
		return m, m.listenForApprovalRequests()

	case DevReloadMsg:
		return m.handleDevReload()

	case commandDoneMsg:
		m.processing = false
		return m, nil

	case quitDoneMsg:
		return m, tea.Quit

	case tickMsg:
		bufName := m.getSelectedBufName()
		logBuf := m.procManager.GetLogBuffer(bufName)
		if logBuf.Follow {
			logBuf.SnapToBottom(m.logViewHeight())
		}
		return m, tickCmd()
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	if m.height < 12 || m.width < 40 {
		return "Terminal too small. Please resize to at least 40×12."
	}

	m.recalcLayout()

	// Pre-compute wrapped visual lines for the log pane (B1)
	if m.scanMode == nil && m.topMode == nil && m.errorStream == nil {
		bufName := m.getSelectedBufName()
		logBuf := m.procManager.GetLogBuffer(bufName)
		contentWidth := m.logWidth - 1 // -1 for leading space
		m.visibleLines = computeVisualLines(logBuf, logBuf.ScrollPos, m.logViewHeight(), contentWidth, m.filterMode)
	} else {
		m.visibleLines = nil
	}

	// Pre-compute sidebar rows once per frame
	if !m.sidebarHidden && m.topMode == nil && m.scanMode == nil {
		m.cachedSidebarRows = buildSidebarRows(&m)
	} else {
		m.cachedSidebarRows = nil
	}

	// Pre-compute error entries once per frame
	if m.errorStream != nil {
		m.cachedErrorEntries = m.buildErrorEntries()
	} else {
		m.cachedErrorEntries = nil
	}

	// Pre-compute top rows once per frame
	if m.topMode != nil {
		m.cachedTopRows = m.buildTopRows()
	} else {
		m.cachedTopRows = nil
	}

	var buf strings.Builder

	// Top border with title
	title := " " + styleBold.Render("devctl") + " "
	titleVisLen := 8 // " devctl "
	topFill := m.width - 2 - 1 - titleVisLen
	if topFill < 0 {
		topFill = 0
	}
	buf.WriteString(boxTL + boxH + title + strings.Repeat(boxH, topFill) + boxTR + "\n")

	// Main content rows
	mainHeight := m.mainHeight()
	for r := 0; r < mainHeight; r++ {
		if m.sidebarHidden {
			var lg string
			if m.errorStream != nil {
				lg = renderErrorStreamRow(&m, r, m.logWidth)
			} else {
				lg = renderLogRow(&m, r, m.logWidth)
			}
			buf.WriteString(boxV + lg + boxV + "\n")
		} else {
			var sb, lg string
			if m.topMode != nil {
				sb = renderTopLeftRow(&m, r, m.sidebarWidth)
				lg = renderTopRightRow(&m, r, m.logWidth)
			} else if m.scanMode != nil {
				sb = renderScanCandidateRow(&m, r, m.sidebarWidth)
				lg = renderScanReadmeRow(&m, r, m.logWidth)
			} else {
				sb = renderSidebar(&m, r, m.sidebarWidth)
				if m.errorStream != nil {
					lg = renderErrorStreamRow(&m, r, m.logWidth)
				} else {
					lg = renderLogRow(&m, r, m.logWidth)
				}
			}
			buf.WriteString(boxV + sb + boxV + lg + boxV + "\n")
		}
	}

	// Divider row
	if m.sidebarHidden {
		buf.WriteString(boxML + strings.Repeat(boxH, m.logWidth) + boxMR + "\n")
	} else {
		buf.WriteString(boxML + strings.Repeat(boxH, m.sidebarWidth) + boxMB + strings.Repeat(boxH, m.logWidth) + boxMR + "\n")
	}

	// Command line row
	cmdContent := m.renderCmdContent(m.width - 2)
	buf.WriteString(boxV + cmdContent + boxV + "\n")

	// Bottom border with hints
	hints := m.getHints()
	hintsLen := len(hints) // approximate; hints don't have ANSI in this version
	hintFill := m.width - 2 - 3 - hintsLen
	if hintFill >= 0 {
		buf.WriteString(boxBL + boxH + " " + hints + " " + strings.Repeat(boxH, hintFill) + boxBR)
	} else {
		buf.WriteString(boxBL + strings.Repeat(boxH, m.width-2) + boxBR)
	}

	return buf.String()
}
