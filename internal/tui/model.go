package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/api"
	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/health"
	"github.com/georgele/devctl/internal/ipc"
	"github.com/georgele/devctl/internal/process"
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
	questionQueue []struct {
		prompt   string
		callback func(string)
	}

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

	// File watch manager
	fileWatchManager *process.FileWatchManager

	// Quitting
	quitting bool

	// Pre-computed visual lines for current render frame (B1)
	visibleLines []visualLine

	// Pre-computed sidebar rows for current render frame
	cachedSidebarRows []sidebarRow

	// Pre-computed error entries for current render frame
	cachedErrorEntries []errorStreamEntry

	// Pre-computed top rows for current render frame
	cachedTopRows []topAppRow

	// Start flags
	startAll bool
	restore  bool

	// HTTP API server
	apiServer     *api.Server
	approvalQueue *api.ApprovalQueue

	// Approval modal
	approvalMode bool
}

// processEventMsg wraps a process event for the Bubble Tea message loop.
type processEventMsg process.ProcessEvent

// processEventBatchMsg wraps multiple process events drained from the channel.
type processEventBatchMsg []process.ProcessEvent

// tickMsg triggers periodic UI refreshes for log output.
type tickMsg time.Time

// clearNotificationMsg clears the hints notification.
type clearNotificationMsg struct{}

// quitDoneMsg signals that all processes have been stopped during quit.
type quitDoneMsg struct{}

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

// fileWatchRestartMsg signals a file change triggering app restart.
type fileWatchRestartMsg struct {
	appName  string
	filePath string
}

// DevReloadMsg signals that the TUI should save state and exit for dev reload.
type DevReloadMsg struct{}

// approvalNotifyMsg signals that a new approval request is pending.
type approvalNotifyMsg struct{}

// portConflictMsg signals port conflicts during start.
type portConflictMsg struct {
	appName   string
	conflicts []struct {
		port  int
		owner *process.PortOwnerInfo
	}
}

// commandDoneMsg signals an async command completed.
type commandDoneMsg struct {
	err error
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

	// Create file watch manager
	m.fileWatchManager = process.NewFileWatchManager()

	// Create config watcher (must be on the model before Init is called)
	configPath := config.ConfigPath(projectRoot)
	if w, err := config.NewWatcher(configPath); err == nil {
		m.configWatcher = w
	}

	// Create IPC server (must be on the model before Init is called)
	if srv, err := ipc.NewServer(projectRoot); err == nil {
		m.ipcServer = srv
	}

	// Create approval queue
	cfg := api.LoadDevctlConfig()
	m.approvalQueue = api.NewApprovalQueue(cfg.Approval)

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

	// Start HTTP API server
	cmds = append(cmds, m.startAPIServer())

	// Listen for approval requests
	if m.approvalQueue != nil {
		cmds = append(cmds, m.listenForApprovalRequests())
	}

	// Start resource alert listener
	cmds = append(cmds, m.listenForResourceAlerts())

	// Start file watch event listener
	cmds = append(cmds, m.listenForFileWatchEvents())

	// Cleanup orphaned processes from previous crash
	cmds = append(cmds, m.cleanupOrphansCmd())

	if m.startAll {
		cmds = append(cmds, m.startAllCmd())
	} else if m.restore {
		cmds = append(cmds, m.restoreSessionCmd())
	} else {
		// Auto-start apps with autoStart: true (independent of --start-all and --restore)
		cmds = append(cmds, m.autoStartCmd())
	}

	return tea.Batch(cmds...)
}
