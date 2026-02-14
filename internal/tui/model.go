package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/health"
	"github.com/georgele/devctl/internal/ipc"
	"github.com/georgele/devctl/internal/process"
	"github.com/georgele/devctl/internal/state"
	"github.com/georgele/devctl/internal/vault"
)

// Focus area
type focusArea int

const (
	focusSidebar focusArea = iota
	focusCommand

	maxCmdHistory  = 100
	quitTimeout    = 10 * time.Second
	notifyDuration = 5 * time.Second
)

// Model is the root Bubble Tea model.
type Model struct {
	// Data
	apps        []config.App
	projectRoot string
	procManager *process.Manager

	// TUI state
	selectedIdx int
	focusArea   focusArea
	width       int
	height      int

	// Layout
	sidebarWidth  int
	logWidth      int
	layoutDirty   bool
	sidebarHidden bool

	// Command line
	cmdInput   string
	cmdCursor  int
	cmdHistory []string
	historyIdx int
	historyTmp string
	processing bool

	// Tab completion
	tabMatches []string
	tabIdx     int
	tabPartial string
	tabOrig    string

	// Search mode
	searchMode *SearchMode

	// Timestamps
	showTimestamps bool

	// Error stream overlay
	errorStream *ErrorStreamMode

	// Question mode
	questionMode *QuestionMode

	// Scan mode
	scanMode *ScanMode

	// Filter mode
	filterMode *FilterMode

	// Notifications
	notification    string
	notificationEnd time.Time

	// Health checker
	healthChecker *health.Checker

	// Resource monitor
	resourceMonitor *process.ResourceMonitor

	// Top mode
	topMode *TopMode

	// Config watcher
	configWatcher *config.Watcher

	// IPC server
	ipcServer *ipc.Server

	// Quitting
	quitting bool

	// Pre-computed visual lines for current render frame (B1)
	visibleLines []visualLine

	// Start flags
	startAll bool
	restore  bool
}

// processEventMsg wraps a process event for the Bubble Tea message loop.
type processEventMsg process.ProcessEvent

// tickMsg triggers periodic UI refreshes for log output.
type tickMsg time.Time

// clearNotificationMsg clears the hints notification.
type clearNotificationMsg struct{}

// autoRestartMsg triggers an auto-restart attempt.
type autoRestartMsg struct {
	appName string
}

// portCheckResultMsg delivers port check results.
type portCheckResultMsg struct {
	results []portCheckResult
}

type portCheckResult struct {
	port    int
	free    bool
	appName string
	owner   *process.PortOwnerInfo
}

// configChangedMsg signals that apps.json was externally modified.
type configChangedMsg struct{}

// ipcRequestMsg wraps an IPC request for the Bubble Tea message loop.
type ipcRequestMsg ipc.IPCRequestMsg

// resourceAlertMsg wraps a resource threshold alert for the Bubble Tea message loop.
type resourceAlertMsg process.ThresholdAlert

// portConflictMsg signals port conflicts during start.
type portConflictMsg struct {
	appName   string
	conflicts []struct {
		port  int
		owner *process.PortOwnerInfo
	}
}

// appEnv returns the resolved environment variables for an app,
// merging vault secrets with plain-text env vars.
func (m *Model) appEnv(env map[string]string, vaultEnv string) map[string]string {
	return m.procManager.ResolveEnv(env, vaultEnv)
}

// New creates a new Model with the given configuration.
func New(projectRoot string, apps []config.App, startAll, restore bool) Model {
	pm := process.NewManager(projectRoot)

	// Set up vault resolver for encrypted env vars
	if vault.Exists(projectRoot) {
		pm.SetVaultResolver(func(root string, plainEnv map[string]string, vaultEnv string) (map[string]string, error) {
			return vault.ResolveEnv(root, plainEnv, vaultEnv)
		})
	}

	// Ensure system log buffer exists
	pm.GetLogBuffer(systemName)

	selectedIdx := 0
	if !startAll && len(apps) > 0 {
		selectedIdx = 1
	}

	m := Model{
		apps:        apps,
		projectRoot: projectRoot,
		procManager: pm,
		selectedIdx: selectedIdx,
		focusArea:   focusCommand,
		historyIdx:  -1,
		startAll:    startAll,
		restore:     restore,
	}

	// Create health checker
	m.healthChecker = health.NewChecker()

	// Create resource monitor
	m.resourceMonitor = process.NewResourceMonitor(pm.PID)

	// Create config watcher (must be on the model before Init is called)
	configPath := config.ConfigPath(projectRoot)
	if w, err := config.NewWatcher(configPath); err == nil {
		m.configWatcher = w
	}

	// Create IPC server (must be on the model before Init is called)
	if srv, err := ipc.NewServer(projectRoot); err == nil {
		m.ipcServer = srv
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.listenForProcessEvents(),
		tickCmd(),
	}

	// Start config watcher
	if m.configWatcher != nil {
		m.configWatcher.Start()
		cmds = append(cmds, m.listenForConfigChange())
	}

	// Start IPC server
	if m.ipcServer != nil {
		m.ipcServer.Start()
		cmds = append(cmds, m.listenForIPCRequests())
	}

	// Start resource alert listener
	cmds = append(cmds, m.listenForResourceAlerts())

	if m.startAll {
		cmds = append(cmds, m.startAllCmd())
	} else if m.restore {
		cmds = append(cmds, m.restoreSessionCmd())
	}

	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutDirty = true
		return m, nil

	case tea.KeyMsg:
		if m.quitting {
			return m, nil
		}
		return m.handleKeypress(msg)

	case processEventMsg:
		cmd := m.processEvent(process.ProcessEvent(msg))
		return m, cmd

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

	case commandDoneMsg:
		m.processing = false
		return m, nil

	case tickMsg:
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

	if m.layoutDirty || m.sidebarWidth == 0 {
		m.recalcLayout()
		m.layoutDirty = false
	}

	// Pre-compute wrapped visual lines for the log pane (B1)
	if m.scanMode == nil && m.topMode == nil && m.errorStream == nil {
		bufName := m.getSelectedBufName()
		logBuf := m.procManager.GetLogBuffer(bufName)
		contentWidth := m.logWidth - 1 // -1 for leading space
		m.visibleLines = computeVisualLines(logBuf, logBuf.ScrollPos, m.logViewHeight(), contentWidth, m.filterMode)
	} else {
		m.visibleLines = nil
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

func (m *Model) recalcLayout() {
	if m.width < 40 || m.height < 12 {
		return
	}

	if m.sidebarHidden {
		m.sidebarWidth = 0
		m.logWidth = m.width - 2
		return
	}

	maxName := 4
	for _, a := range m.apps {
		if len(a.Name) > maxName {
			maxName = len(a.Name)
		}
	}

	m.sidebarWidth = maxName + 6
	if m.sidebarWidth < 16 {
		m.sidebarWidth = 16
	}
	maxSidebar := m.width * 35 / 100
	if m.sidebarWidth > maxSidebar {
		m.sidebarWidth = maxSidebar
	}

	m.logWidth = m.width - m.sidebarWidth - 3
}

func (m *Model) mainHeight() int {
	// height - top border (1) - divider (1) - command (1) - bottom border (1)
	h := m.height - 4
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) logViewHeight() int {
	return m.mainHeight() - 1 // minus header row
}

func (m *Model) getSelectedApp() *config.App {
	if m.selectedIdx == 0 {
		return nil
	}
	idx := m.selectedIdx - 1
	if idx < 0 || idx >= len(m.apps) {
		return nil
	}
	return &m.apps[idx]
}

func (m *Model) getSelectedBufName() string {
	if m.selectedIdx == 0 {
		return systemName
	}
	idx := m.selectedIdx - 1
	if idx >= 0 && idx < len(m.apps) {
		return m.apps[idx].Name
	}
	return systemName
}

func (m *Model) saveConfig() {
	if m.configWatcher != nil {
		m.configWatcher.SetIgnoreNext()
	}
	if err := config.Save(m.projectRoot, m.apps); err != nil {
		m.systemLog(fmt.Sprintf("Error saving config: %s", err))
	}
}

func (m *Model) systemLog(msg string) {
	buf := m.procManager.GetLogBuffer(systemName)
	buf.Append(msg, false)
	if m.selectedIdx == 0 && buf.Follow {
		buf.SnapToBottom(m.logViewHeight())
	}
}

func (m *Model) renderCmdContent(width int) string {
	if m.questionMode != nil {
		content := styleBold.Render(m.questionMode.Prompt) + m.questionMode.Input
		return padRight(content, width)
	}

	if m.searchMode != nil {
		count := len(m.searchMode.matches)
		pos := 0
		if m.searchMode.matchIdx >= 0 {
			pos = m.searchMode.matchIdx + 1
		}
		countStr := fmt.Sprintf(" [%d/%d]", pos, count)
		if count == 0 {
			countStr = " [no matches]"
		}
		content := styleBold.Render("/") + m.searchMode.pattern + styleDim.Render(countStr)
		return padRight(content, width)
	}

	// Show filter indicator
	filterIndicator := ""
	if m.filterMode != nil && m.filterMode.pattern != "" {
		filterIndicator = styleDim.Render(" [filter: "+m.filterMode.pattern+"]")
	}
	if m.errorStream != nil {
		filterIndicator += styleDim.Render(" [error stream]")
	}

	var prompt string
	if m.focusArea == focusCommand {
		prompt = styleBold.Render("devctl>") + " "
	} else {
		prompt = styleDim.Render("devctl>") + " "
	}
	return padRight(prompt+m.cmdInput+filterIndicator, width)
}

func (m *Model) getHints() string {
	// Show time-limited notification if active
	if m.notification != "" && time.Now().Before(m.notificationEnd) {
		return m.notification
	}

	if m.errorStream != nil {
		return "x/Esc: back | Enter: expand | e: copy | m: msg | l: loc | j/k: nav | c: clear"
	}

	if m.topMode != nil {
		return "c: CPU | m: MEM | n: name | u: uptime | r: reverse | Esc/q: exit"
	}

	if m.scanMode != nil {
		return "Space: toggle | a: all/none | Tab: focus | Enter: confirm | Esc: cancel"
	}

	if m.questionMode != nil {
		return "Enter: submit | Esc: cancel"
	}

	if m.searchMode != nil {
		count := len(m.searchMode.matches)
		pos := 0
		if m.searchMode.matchIdx >= 0 {
			pos = m.searchMode.matchIdx + 1
		}
		return fmt.Sprintf("Enter/Esc: exit | n: next | N: prev | %d/%d matches", pos, count)
	}

	// Check for errors on selected app
	hasErrors := false
	if app := m.getSelectedApp(); app != nil {
		hasErrors = m.procManager.GetErrorCount(app.Name) > 0
	}

	if m.focusArea == focusSidebar {
		hint := "Tab: command | up/down/jk: nav | s/S/r: start/stop/restart | R: all | p: pin | x: error stream | ^J/K: scroll | ^B: sidebar | ^C: quit"
		if hasErrors {
			hint = "e: copy error | E: copy all | " + hint
		}
		return hint
	}
	hint := "Tab: sidebar | /: search | f: filter | t: timestamps | x: error stream | up/down: history | ^J/K: scroll | ^B: sidebar | ^C: quit"
	if hasErrors {
		hint = "e: copy error | E: copy all | " + hint
	}
	return hint
}

// handleKeypress dispatches key events to the appropriate handler.
func (m Model) handleKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C → quit
	if isCtrl(msg, "c") {
		return m.handleQuit()
	}

	// Error stream mode
	if m.errorStream != nil {
		return m.handleErrorStreamKeypress(msg)
	}

	// Top mode
	if m.topMode != nil {
		return m.handleTopKeypress(msg)
	}

	// Scan mode
	if m.scanMode != nil {
		return m.handleScanKeypress(msg)
	}

	// Question mode
	if m.questionMode != nil {
		return m.handleQuestionKeypress(msg)
	}

	// Search mode
	if m.searchMode != nil {
		return m.handleSearchKeypress(msg)
	}

	// Ctrl+B: toggle sidebar (global)
	if isCtrl(msg, "b") {
		m.sidebarHidden = !m.sidebarHidden
		m.layoutDirty = true
		return m, nil
	}

	// PageUp/PageDown in any mode
	if isKey(msg, "pgup") {
		m.scrollLog(-(m.logViewHeight() - 1))
		return m, nil
	}
	if isKey(msg, "pgdown") {
		m.scrollLog(m.logViewHeight() - 1)
		return m, nil
	}

	// Ctrl+K/J: scroll log by one line
	if isCtrl(msg, "k") {
		m.scrollLog(-1)
		return m, nil
	}
	if isCtrl(msg, "j") {
		m.scrollLog(1)
		return m, nil
	}

	if m.focusArea == focusSidebar {
		return m.handleSidebarKeypress(msg)
	}
	return m.handleCommandKeypress(msg)
}

func (m Model) handleSidebarKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "tab"):
		m.focusArea = focusCommand
		return m, nil

	case isKey(msg, "up", "k"):
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
		return m, nil

	case isKey(msg, "down", "j"):
		if m.selectedIdx < len(m.apps) {
			m.selectedIdx++
		}
		return m, nil

	case isKey(msg, "enter"):
		m.focusArea = focusCommand
		return m, nil

	case isRune(msg, 'R'):
		return m, m.executeAsync("restart", "all")

	case isRune(msg, 's'):
		if app := m.getSelectedApp(); app != nil {
			return m, m.executeAsync("start", app.Name)
		}
		return m, nil

	case isRune(msg, 'S'):
		if app := m.getSelectedApp(); app != nil {
			return m, m.executeAsync("stop", app.Name)
		}
		return m, nil

	case isRune(msg, 'r'):
		if app := m.getSelectedApp(); app != nil {
			return m, m.executeAsync("restart", app.Name)
		}
		return m, nil

	case isRune(msg, 'p'):
		if app := m.getSelectedApp(); app != nil {
			pinned := !(app.Pinned != nil && *app.Pinned)
			app.Pinned = &pinned
			m.layoutDirty = true
			m.saveConfig()
			if pinned {
				m.systemLog(fmt.Sprintf("Pinned %s", app.Name))
			} else {
				m.systemLog(fmt.Sprintf("Unpinned %s", app.Name))
			}
		}
		return m, nil

	case isRune(msg, 'x'):
		allApps := m.selectedIdx == 0
		m.errorStream = newErrorStreamMode(allApps)
		return m, nil

	case isRune(msg, 'e'):
		return m.copyLastError()

	case isRune(msg, 'E'):
		return m.copyAllErrors()
	}

	// Any printable key → switch to command mode
	if msg.Type == tea.KeyRunes {
		m.focusArea = focusCommand
		return m.handleCommandKeypress(msg)
	}

	return m, nil
}

func (m Model) handleCommandKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "tab"):
		if m.cmdInput == "" {
			m.focusArea = focusSidebar
			return m, nil
		}
		m.handleTabCompletion()
		return m, nil

	case isKey(msg, "enter"):
		if m.processing || m.cmdInput == "" {
			return m, nil
		}
		return m.executeCommandLine()

	case isKey(msg, "up"):
		m.navigateHistory(-1)
		return m, nil

	case isKey(msg, "down"):
		m.navigateHistory(1)
		return m, nil

	case isKey(msg, "backspace"):
		if m.cmdCursor > 0 {
			m.cmdInput = m.cmdInput[:m.cmdCursor-1] + m.cmdInput[m.cmdCursor:]
			m.cmdCursor--
		}
		m.tabMatches = nil
		return m, nil

	case isKey(msg, "delete"):
		if m.cmdCursor < len(m.cmdInput) {
			m.cmdInput = m.cmdInput[:m.cmdCursor] + m.cmdInput[m.cmdCursor+1:]
		}
		m.tabMatches = nil
		return m, nil

	case isKey(msg, "left"):
		if m.cmdCursor > 0 {
			m.cmdCursor--
		}
		return m, nil

	case isKey(msg, "right"):
		if m.cmdCursor < len(m.cmdInput) {
			m.cmdCursor++
		}
		return m, nil

	case isKey(msg, "home") || isCtrl(msg, "a"):
		m.cmdCursor = 0
		return m, nil

	case isKey(msg, "end") || isCtrl(msg, "e"):
		m.cmdCursor = len(m.cmdInput)
		return m, nil

	case isCtrl(msg, "u"):
		m.cmdInput = ""
		m.cmdCursor = 0
		return m, nil

	case isCtrl(msg, "w"):
		if m.cmdCursor > 0 {
			before := m.cmdInput[:m.cmdCursor]
			after := m.cmdInput[m.cmdCursor:]
			// Remove last word
			trimmed := strings.TrimRight(before, " ")
			lastSpace := strings.LastIndexByte(trimmed, ' ')
			if lastSpace >= 0 {
				before = trimmed[:lastSpace+1]
			} else {
				before = ""
			}
			m.cmdInput = before + after
			m.cmdCursor = len(before)
		}
		return m, nil
	}

	// "/" or Ctrl+F: search mode (when input is empty)
	if (isRune(msg, '/') || isCtrl(msg, "f")) && m.cmdInput == "" {
		m.searchMode = newSearchMode()
		return m, nil
	}

	// "e"/"E": copy errors (when input is empty)
	if isRune(msg, 'e') && m.cmdInput == "" {
		return m.copyLastError()
	}
	if isRune(msg, 'E') && m.cmdInput == "" {
		return m.copyAllErrors()
	}

	// "f": toggle filter mode (when input is empty)
	if isRune(msg, 'f') && m.cmdInput == "" {
		if m.filterMode != nil {
			m.filterMode = nil
			m.systemLog("Filter disabled")
		} else {
			m.filterMode = newFilterMode()
			m.systemLog("Filter mode: type pattern, press f again to disable")
		}
		return m, nil
	}

	// "t": toggle timestamps (when input is empty)
	if isRune(msg, 't') && m.cmdInput == "" {
		m.showTimestamps = !m.showTimestamps
		if m.showTimestamps {
			m.systemLog("Timestamps enabled")
		} else {
			m.systemLog("Timestamps disabled")
		}
		return m, nil
	}

	// "x": open error stream (when input is empty)
	if isRune(msg, 'x') && m.cmdInput == "" {
		allApps := m.selectedIdx == 0
		m.errorStream = newErrorStreamMode(allApps)
		return m, nil
	}

	// Regular character input
	if msg.Type == tea.KeyRunes {
		m.tabMatches = nil
		ch := string(msg.Runes)
		m.cmdInput = m.cmdInput[:m.cmdCursor] + ch + m.cmdInput[m.cmdCursor:]
		m.cmdCursor += len(ch)
		return m, nil
	}

	return m, nil
}

func (m Model) handleSearchKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "esc", "enter"):
		m.searchMode = nil
		return m, nil

	case isRune(msg, 'n'):
		m.searchMode.navigate(1)
		m.scrollToSearchMatch()
		return m, nil

	case isRune(msg, 'N'):
		m.searchMode.navigate(-1)
		m.scrollToSearchMatch()
		return m, nil

	case isKey(msg, "backspace"):
		if len(m.searchMode.pattern) > 0 {
			m.searchMode.pattern = m.searchMode.pattern[:len(m.searchMode.pattern)-1]
			logBuf := m.procManager.GetLogBuffer(m.getSelectedBufName())
			m.searchMode.updateMatches(logBuf)
		}
		return m, nil

	case isCtrl(msg, "u"):
		m.searchMode.pattern = ""
		logBuf := m.procManager.GetLogBuffer(m.getSelectedBufName())
		m.searchMode.updateMatches(logBuf)
		return m, nil
	}

	// Regular character → add to search pattern
	if msg.Type == tea.KeyRunes {
		m.searchMode.pattern += string(msg.Runes)
		logBuf := m.procManager.GetLogBuffer(m.getSelectedBufName())
		m.searchMode.updateMatches(logBuf)
		if len(m.searchMode.matches) > 0 && m.searchMode.matchIdx >= 0 {
			m.scrollToSearchMatch()
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) scrollLog(delta int) {
	bufName := m.getSelectedBufName()
	logBuf := m.procManager.GetLogBuffer(bufName)
	logBuf.ScrollBy(delta, m.logViewHeight())
}

func (m *Model) scrollToSearchMatch() {
	if m.searchMode == nil {
		return
	}
	match := m.searchMode.currentMatch()
	if match == nil {
		return
	}
	bufName := m.getSelectedBufName()
	logBuf := m.procManager.GetLogBuffer(bufName)
	viewHeight := m.logViewHeight()

	if match.LineIdx < logBuf.ScrollPos {
		logBuf.ScrollTo(match.LineIdx, viewHeight)
	} else if match.LineIdx >= logBuf.ScrollPos+viewHeight {
		logBuf.ScrollTo(match.LineIdx-viewHeight+1, viewHeight)
	}
}

func (m *Model) navigateHistory(dir int) {
	if len(m.cmdHistory) == 0 {
		return
	}
	if dir < 0 {
		if m.historyIdx == -1 {
			m.historyTmp = m.cmdInput
			m.historyIdx = len(m.cmdHistory) - 1
		} else if m.historyIdx > 0 {
			m.historyIdx--
		} else {
			return
		}
		m.cmdInput = m.cmdHistory[m.historyIdx]
	} else {
		if m.historyIdx == -1 {
			return
		}
		if m.historyIdx < len(m.cmdHistory)-1 {
			m.historyIdx++
			m.cmdInput = m.cmdHistory[m.historyIdx]
		} else {
			m.historyIdx = -1
			m.cmdInput = m.historyTmp
		}
	}
	m.cmdCursor = len(m.cmdInput)
}

func (m *Model) handleTabCompletion() {
	if m.tabMatches == nil {
		matches, partial := m.complete(m.cmdInput[:m.cmdCursor])
		if len(matches) == 0 {
			return
		}

		if len(matches) == 1 {
			before := m.cmdInput[:m.cmdCursor-len(partial)]
			after := m.cmdInput[m.cmdCursor:]
			m.cmdInput = before + matches[0] + " " + after
			m.cmdCursor = len(before) + len(matches[0]) + 1
			return
		}

		cp := commonPrefix(matches)
		if len(cp) > len(partial) {
			before := m.cmdInput[:m.cmdCursor-len(partial)]
			after := m.cmdInput[m.cmdCursor:]
			m.cmdInput = before + cp + after
			m.cmdCursor = len(before) + len(cp)
		}

		m.tabMatches = matches
		m.tabIdx = 0
		m.tabPartial = partial
		m.tabOrig = m.cmdInput
		m.systemLog("Completions: " + strings.Join(matches, "  "))
		return
	}

	// Cycle through matches
	m.tabIdx = (m.tabIdx + 1) % len(m.tabMatches)
	match := m.tabMatches[m.tabIdx]
	before := m.tabOrig[:len(m.tabOrig)-len(m.tabPartial)]
	m.cmdInput = before + match
	m.cmdCursor = len(m.cmdInput)
}

// executeCommandLine processes the current command input.
func (m Model) executeCommandLine() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.cmdInput)
	if line == "" {
		return m, nil
	}

	m.cmdHistory = append(m.cmdHistory, line)
	if len(m.cmdHistory) > maxCmdHistory {
		m.cmdHistory = m.cmdHistory[1:]
	}
	m.historyIdx = -1
	m.historyTmp = ""
	m.cmdInput = ""
	m.cmdCursor = 0
	m.tabMatches = nil

	cmd, args := parseCommand(line)
	return m.dispatchCommand(cmd, args)
}

// dispatchCommand routes a command to its handler.
func (m Model) dispatchCommand(cmd, args string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "start":
		if args == "" {
			m.systemLog("Usage: start <name|all>")
			return m, nil
		}
		return m, m.executeAsync("start", args)

	case "stop":
		if args == "" {
			m.systemLog("Usage: stop <name|all>")
			return m, nil
		}
		return m, m.executeAsync("stop", args)

	case "restart":
		if args == "" {
			m.systemLog("Usage: restart <name|all>")
			return m, nil
		}
		return m, m.executeAsync("restart", args)

	case "status":
		m.showStatus(args)
		return m, nil

	case "list":
		m.showList()
		return m, nil

	case "help":
		m.showHelp()
		return m, nil

	case "quit", "exit":
		return m.handleQuit()

	case "reload":
		return m.handleReload()

	case "scan":
		return m, m.scanAsync()

	case "add":
		m.startAddWizard()
		return m, nil

	case "remove":
		return m.handleRemove(args)

	case "ports":
		return m, m.checkPortsAsync()

	case "autorestart":
		m.handleAutoRestart(args)
		return m, nil

	case "clear-errors":
		m.handleClearErrors(args)
		return m, nil

	case "export":
		m.handleExport(args)
		return m, nil

	case "pin":
		m.handlePin(args, true)
		return m, nil

	case "unpin":
		m.handlePin(args, false)
		return m, nil

	case "run":
		return m, m.handleRun(args)

	case "top":
		m.topMode = newTopMode()
		return m, nil

	default:
		m.systemLog(fmt.Sprintf("Unknown command: %s. Type 'help' for available commands.", cmd))
		return m, nil
	}
}

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
	m.layoutDirty = true
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

	return func() tea.Msg {
		m.procManager.Restart(app.Name, cmd, app.Dir, m.appEnv(app.Env, app.VaultEnv))
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

// startWithPortCheck starts an app with port conflict detection.
func (m *Model) startWithPortCheck(app *config.App) tea.Cmd {
	return func() tea.Msg {
		// Check all ports
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

		if len(taken) == 0 {
			// No conflicts, just start
			m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			return commandDoneMsg{}
		}

		// Handle conflicts - log info and ask user
		for _, t := range taken {
			logBuf := m.procManager.GetLogBuffer(app.Name)
			if t.owner == nil {
				logBuf.Append(fmt.Sprintf("Port %d is in use by unknown process", t.port), false)
			} else {
				devctlApp := m.procManager.FindDevctlOwner(t.owner.PID)
				if devctlApp != "" {
					logBuf.Append(fmt.Sprintf("Port %d is used by devctl app \"%s\" (running)", t.port, devctlApp), false)
					logBuf.Append("Options:", false)
					logBuf.Append("  [r] Restart "+devctlApp+", then start this app", false)
					altPort := process.SuggestAlternativePort(t.port)
					if altPort > 0 {
						logBuf.Append(fmt.Sprintf("  [a] Use alternative port (%d is free)", altPort), false)
					}
					logBuf.Append("  [s] Start anyway (may fail)", false)
					logBuf.Append("  [c] Cancel", false)
				} else {
					logBuf.Append(fmt.Sprintf("Port %d is in use by external process:", t.port), false)
					logBuf.Append(fmt.Sprintf("  PID: %d, Command: %s, User: %s", t.owner.PID, t.owner.Command, t.owner.User), false)
					logBuf.Append("Options:", false)
					logBuf.Append("  [k] Kill the process and start", false)
					altPort := process.SuggestAlternativePort(t.port)
					if altPort > 0 {
						logBuf.Append(fmt.Sprintf("  [a] Use alternative port (%d is free)", altPort), false)
					}
					logBuf.Append("  [s] Start anyway (may fail)", false)
					logBuf.Append("  [c] Cancel", false)
				}
			}
		}

		// Use a question prompt for resolution
		// This will be handled via the TUI's question mode
		return portConflictMsg{appName: app.Name, conflicts: taken}
	}
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
					m.layoutDirty = true
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
	m.layoutDirty = true
	m.saveConfig()
	// Clean up stale entries from the process manager
	m.procManager.RemoveEntries(name)
	m.systemLog(fmt.Sprintf("Removed %s.", name))
	if m.selectedIdx > len(m.apps) {
		m.selectedIdx = len(m.apps)
	}
}

func (m Model) checkPortsAsync() tea.Cmd {
	return func() tea.Msg {
		var results []portCheckResult
		for _, app := range m.apps {
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
	return func() tea.Msg {
		candidates := config.DetectApps(m.projectRoot, m.apps)
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
	// Save session state BEFORE stopping (so we know which apps were running)
	// Always save, even if empty, to clear stale sessions (B10)
	var running []string
	for _, app := range m.apps {
		if m.procManager.GetStatus(app.Name) == process.StatusRunning {
			running = append(running, app.Name)
		}
	}
	state.SaveSession(m.projectRoot, running)
	// Now stop all processes with a timeout
	done := make(chan struct{})
	go func() {
		m.procManager.StopAll()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(quitTimeout):
		// Force quit if processes refuse to die
	}
	return m, tea.Quit
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

	m.apps = newApps
	m.layoutDirty = true
	if m.selectedIdx > len(m.apps) {
		m.selectedIdx = len(m.apps)
	}
	m.systemLog("Config reloaded successfully.")
	return m, nil
}

// commandDoneMsg signals an async command completed.
type commandDoneMsg struct {
	err error
}

func (m Model) executeAsync(action, target string) tea.Cmd {
	// Expand @group targets
	if strings.HasPrefix(target, "@") {
		names := m.expandGroupTarget(target)
		if len(names) == 0 {
			return nil
		}
		return func() tea.Msg {
			for _, name := range names {
				app := m.findApp(name)
				if app == nil {
					continue
				}
				switch action {
				case "start":
					m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
				case "stop":
					m.procManager.Stop(app.Name)
				case "restart":
					m.procManager.Restart(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
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
				sorted, sortErr := config.TopologicalSort(m.apps)
				if sortErr != nil {
					m.systemLog(fmt.Sprintf("Dependency warning: %s — starting in config order", sortErr))
					sorted = m.apps
				}
				// Pre-check ports for all apps, start conflict-free ones in order
				type conflictInfo struct {
					appName string
					port    int
					owner   *process.PortOwnerInfo
				}
				var conflicts []conflictInfo
				for _, app := range sorted {
					status := m.procManager.GetStatus(app.Name)
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
						m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
					}
				}
				// Return conflict messages for individual resolution (B2)
				if len(conflicts) > 0 {
					names := make([]string, len(conflicts))
					for i, c := range conflicts {
						names[i] = c.appName
						logBuf := m.procManager.GetLogBuffer(c.appName)
						if c.owner != nil {
							logBuf.Append(fmt.Sprintf("Port %d in use by %s (PID %d)", c.port, c.owner.Command, c.owner.PID), false)
						} else {
							logBuf.Append(fmt.Sprintf("Port %d in use", c.port), false)
						}
					}
					m.systemLog(fmt.Sprintf("Port conflicts: %s — use 'start <name>' to resolve individually", strings.Join(names, ", ")))
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
				app := m.findApp(target)
				if app == nil {
					m.systemLog(fmt.Sprintf("Unknown app: %s", target))
				} else {
					// Auto-start dependencies first
					deps := config.DependencyOrder(m.apps, target)
					for _, depName := range deps {
						if m.procManager.GetStatus(depName) != process.StatusRunning {
							depApp := m.findApp(depName)
							if depApp != nil {
								m.systemLog(fmt.Sprintf("Starting dependency %s for %s", depName, target))
								m.procManager.Start(depApp.Name, depApp.Command, depApp.Dir, m.appEnv(depApp.Env, depApp.VaultEnv))
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
					m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
				}
			}
		case "stop":
			if target == "all" {
				m.procManager.StopAll()
			} else {
				m.procManager.Stop(target)
			}
		case "restart":
			if target == "all" {
				for _, app := range m.apps {
					m.procManager.Stop(app.Name)
				}
				for _, app := range m.apps {
					for _, p := range app.Ports {
						process.WaitForPortFree(p, 3*time.Second)
					}
				}
				for _, app := range m.apps {
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
					m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
				}
			} else {
				app := m.findApp(target)
				if app == nil {
					m.systemLog(fmt.Sprintf("Unknown app: %s", target))
				} else {
					m.procManager.Stop(app.Name)
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
					m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
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
	return func() tea.Msg {
		sorted, err := config.TopologicalSort(m.apps)
		if err != nil {
			m.systemLog(fmt.Sprintf("Dependency warning: %s — starting in config order", err))
			sorted = m.apps
		}
		for _, app := range sorted {
			m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
		}
		return commandDoneMsg{}
	}
}

func (m Model) restoreSessionCmd() tea.Cmd {
	return func() tea.Msg {
		saved := state.LoadSession(m.projectRoot)
		if len(saved) == 0 {
			m.systemLog("No previous session to restore.")
			return commandDoneMsg{}
		}

		appMap := make(map[string]config.App)
		for _, a := range m.apps {
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
			m.systemLog(fmt.Sprintf("Dependency warning: %s — restoring in saved order", sortErr))
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
					logBuf := m.procManager.GetLogBuffer(app.Name)
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
			m.procManager.Start(app.Name, app.Command, app.Dir, m.appEnv(app.Env, app.VaultEnv))
			restored++
		}
		if len(skippedConflicts) > 0 {
			m.systemLog(fmt.Sprintf("Port conflicts during restore: %s — use 'start <name>' to resolve individually", strings.Join(skippedConflicts, ", ")))
		}

		state.ClearSession(m.projectRoot)
		m.systemLog(fmt.Sprintf("Restored %d app(s) from previous session.", restored))
		if len(missing) > 0 {
			m.systemLog(fmt.Sprintf("Warning: %d app(s) from previous session no longer in config: %s", len(missing), strings.Join(missing, ", ")))
		}
		return commandDoneMsg{}
	}
}

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
		return tea.Batch(
			m.listenForProcessEvents(),
			tea.Tick(notifyDuration, func(time.Time) tea.Msg { return clearNotificationMsg{} }),
		)
	}

	// Desktop notification and auto-restart on crash
	if evt.Type == process.EventCrashed && !m.quitting {
		// Send desktop notification if enabled for this app
		if app := m.findApp(evt.AppName); app != nil && app.Notifications != nil && *app.Notifications {
			go process.SendNotification("devctl", fmt.Sprintf("%s crashed (code %d)", evt.AppName, evt.Code))
		}

		restartCmd := m.maybeAutoRestart(evt.AppName)
		if restartCmd != nil {
			return tea.Batch(m.listenForProcessEvents(), restartCmd)
		}
	}

	return m.listenForProcessEvents()
}

func (m Model) listenForProcessEvents() tea.Cmd {
	return func() tea.Msg {
		evt := <-m.procManager.Events()
		return processEventMsg(evt)
	}
}

func (m Model) listenForResourceAlerts() tea.Cmd {
	ch := m.resourceMonitor.Alerts()
	return func() tea.Msg {
		alert := <-ch
		return resourceAlertMsg(alert)
	}
}

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
	m.layoutDirty = true
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
