package tui

import (
	"fmt"
	"time"

	"github.com/georgele/hum/internal/config"
)

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
	if m.approvalMode && m.approvalQueue != nil && m.approvalQueue.PendingCount() > 0 {
		pending := m.approvalQueue.Pending()
		req := pending[0]
		remaining := time.Until(req.Deadline).Truncate(time.Second)
		if remaining < 0 {
			remaining = 0
		}
		badge := ""
		if len(pending) > 1 {
			badge = fmt.Sprintf(" [%d queued]", len(pending)-1)
		}
		content := fmt.Sprintf("[%s] %s %s from %s (%ds)%s [y/n/s]",
			styleBold.Render("APPROVE?"),
			req.Action, req.Detail, req.ClientName,
			int(remaining.Seconds()), badge)
		return padRight(content, width)
	}

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
		prompt = styleBold.Render("humrun>") + " "
	} else {
		prompt = styleDim.Render("humrun>") + " "
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

	if m.approvalMode && m.approvalQueue != nil && m.approvalQueue.PendingCount() > 0 {
		return "y: approve | n: deny | s: skip | Esc: dismiss"
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
