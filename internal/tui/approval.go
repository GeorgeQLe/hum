package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/api"
)

// listenForApprovalRequests listens for new approval requests from the API.
func (m Model) listenForApprovalRequests() tea.Cmd {
	if m.approvalQueue == nil {
		return nil
	}
	ch := m.approvalQueue.Notify()
	return func() tea.Msg {
		<-ch
		return approvalNotifyMsg{}
	}
}

// handleApprovalKeypress processes keypresses in approval mode.
func (m Model) handleApprovalKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "y"):
		m.approvalQueue.Decide(api.DecisionApproved)
		if m.approvalQueue.PendingCount() == 0 {
			m.approvalMode = false
		}
		return m, nil
	case isKey(msg, "n"):
		m.approvalQueue.Decide(api.DecisionDenied)
		if m.approvalQueue.PendingCount() == 0 {
			m.approvalMode = false
		}
		return m, nil
	case isKey(msg, "s"):
		m.approvalQueue.Decide(api.DecisionSkipped)
		if m.approvalQueue.PendingCount() == 0 {
			m.approvalMode = false
		}
		return m, nil
	case isKey(msg, "esc"):
		// Dismiss modal without deciding (request will timeout)
		m.approvalMode = false
		return m, nil
	}
	return m, nil
}
