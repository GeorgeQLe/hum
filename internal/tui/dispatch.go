package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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

	case "watch":
		m.handleWatch(args)
		return m, nil

	default:
		m.systemLog(fmt.Sprintf("Unknown command: %s. Type 'help' for available commands.", cmd))
		return m, nil
	}
}
