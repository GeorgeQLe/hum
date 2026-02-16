package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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

	// Approval mode
	if m.approvalMode && m.approvalQueue != nil && m.approvalQueue.PendingCount() > 0 {
		return m.handleApprovalKeypress(msg)
	}

	// Question mode
	if m.questionMode != nil {
		return m.handleQuestionKeypress(msg)
	}

	// Search mode
	if m.searchMode != nil {
		return m.handleSearchKeypress(msg)
	}

	// Filter mode
	if m.filterMode != nil {
		return m.handleFilterKeypress(msg)
	}

	// Ctrl+B: toggle sidebar (global)
	if isCtrl(msg, "b") {
		m.sidebarHidden = !m.sidebarHidden
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
