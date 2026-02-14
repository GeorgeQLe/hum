package envsafetui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/georgele/devctl/internal/vault"
	"github.com/georgele/devctl/internal/vault/audit"
)

// View represents the current TUI screen.
type View int

const (
	ViewEnvironments View = iota
	ViewSecrets
	ViewSecretDetail
	ViewAuditLog
)

var (
	colorGreen  = lipgloss.Color("2")
	colorYellow = lipgloss.Color("3")
	colorCyan   = lipgloss.Color("6")

	styleBold      = lipgloss.NewStyle().Bold(true)
	styleDim       = lipgloss.NewStyle().Faint(true)
	styleHighlight = lipgloss.NewStyle().Reverse(true)
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	styleKey       = lipgloss.NewStyle().Foreground(colorGreen)
	styleValue     = lipgloss.NewStyle().Foreground(colorYellow)

	boxH = "─"
	boxV = "│"
)

// Model is the Bubble Tea model for envsafe browse.
type Model struct {
	vault       *vault.Vault
	auditLogger *audit.Logger

	view        View
	width       int
	height      int

	// Environment list
	environments []string
	envIdx       int

	// Secret list
	currentEnv  string
	secrets     []string
	secretIdx   int

	// Secret detail
	currentKey  string
	showValue   bool

	// Audit log
	auditEntries []audit.Entry
	auditIdx     int
	auditScroll  int

	quitting bool
}

// New creates a new envsafe TUI model.
func New(v *vault.Vault, logger *audit.Logger) Model {
	envs := v.ListEnvironments()
	return Model{
		vault:        v,
		auditLogger:  logger,
		environments: envs,
		view:         ViewEnvironments,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "tab":
			// Cycle views
			switch m.view {
			case ViewEnvironments:
				m.view = ViewAuditLog
				m.loadAudit()
			case ViewSecrets:
				m.view = ViewEnvironments
			case ViewSecretDetail:
				m.view = ViewSecrets
			case ViewAuditLog:
				m.view = ViewEnvironments
			}
			return m, nil

		case "enter":
			return m.handleEnter()

		case "esc", "backspace":
			return m.handleBack()

		case "up", "k":
			return m.handleUp()

		case "down", "j":
			return m.handleDown()

		case "s":
			if m.view == ViewSecretDetail {
				m.showValue = !m.showValue
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	var content string
	switch m.view {
	case ViewEnvironments:
		content = m.viewEnvironments()
	case ViewSecrets:
		content = m.viewSecrets()
	case ViewSecretDetail:
		content = m.viewSecretDetail()
	case ViewAuditLog:
		content = m.viewAuditLog()
	}

	// Header
	header := styleTitle.Render("envsafe") + styleDim.Render(" — encrypted secrets browser")
	footer := m.viewFooter()

	return header + "\n" + strings.Repeat(boxH, min(m.width, 60)) + "\n" + content + "\n" + footer
}

func (m *Model) viewEnvironments() string {
	var b strings.Builder
	b.WriteString(styleBold.Render("Environments") + "\n\n")

	for i, env := range m.environments {
		cursor := "  "
		if i == m.envIdx {
			cursor = styleHighlight.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, env))
	}

	return b.String()
}

func (m *Model) viewSecrets() string {
	var b strings.Builder
	b.WriteString(styleBold.Render("Secrets — "+m.currentEnv) + "\n\n")

	if len(m.secrets) == 0 {
		b.WriteString(styleDim.Render("  (no secrets)") + "\n")
		return b.String()
	}

	for i, key := range m.secrets {
		cursor := "  "
		if i == m.secretIdx {
			cursor = styleHighlight.Render("> ")
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, styleKey.Render(key)))
	}

	return b.String()
}

func (m *Model) viewSecretDetail() string {
	var b strings.Builder
	b.WriteString(styleBold.Render(m.currentEnv+"/"+m.currentKey) + "\n\n")

	value, err := m.vault.Get(m.currentEnv, m.currentKey)
	if err != nil {
		b.WriteString(fmt.Sprintf("  Error: %v\n", err))
		return b.String()
	}

	if m.showValue {
		b.WriteString(fmt.Sprintf("  Value: %s\n", styleValue.Render(value)))
	} else {
		b.WriteString(fmt.Sprintf("  Value: %s\n", styleDim.Render("••••••••  (press 's' to reveal)")))
	}

	return b.String()
}

func (m *Model) viewAuditLog() string {
	var b strings.Builder
	b.WriteString(styleBold.Render("Audit Log") + "\n\n")

	if len(m.auditEntries) == 0 {
		b.WriteString(styleDim.Render("  (no audit entries)") + "\n")
		return b.String()
	}

	maxVisible := m.height - 8
	if maxVisible < 5 {
		maxVisible = 5
	}

	end := m.auditScroll + maxVisible
	if end > len(m.auditEntries) {
		end = len(m.auditEntries)
	}

	for i := m.auditScroll; i < end; i++ {
		e := m.auditEntries[i]
		ts := e.Timestamp.Format("01-02 15:04")
		cursor := "  "
		if i == m.auditIdx {
			cursor = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s %-8s %-15s %s/%s\n",
			cursor, styleDim.Render(ts), e.Action, e.User, e.Environment, e.Key))
	}

	return b.String()
}

func (m *Model) viewFooter() string {
	switch m.view {
	case ViewEnvironments:
		return styleDim.Render("↑↓ navigate • enter select • tab audit • q quit")
	case ViewSecrets:
		return styleDim.Render("↑↓ navigate • enter view • esc back • q quit")
	case ViewSecretDetail:
		return styleDim.Render("s toggle value • esc back • q quit")
	case ViewAuditLog:
		return styleDim.Render("↑↓ scroll • tab environments • q quit")
	}
	return ""
}

func (m Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewEnvironments:
		if len(m.environments) > 0 {
			m.currentEnv = m.environments[m.envIdx]
			keys, _ := m.vault.List(m.currentEnv)
			m.secrets = keys
			m.secretIdx = 0
			m.view = ViewSecrets
		}
	case ViewSecrets:
		if len(m.secrets) > 0 {
			m.currentKey = m.secrets[m.secretIdx]
			m.showValue = false
			m.view = ViewSecretDetail
		}
	}
	return m, nil
}

func (m Model) handleBack() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewSecrets:
		m.view = ViewEnvironments
	case ViewSecretDetail:
		m.view = ViewSecrets
	case ViewAuditLog:
		m.view = ViewEnvironments
	}
	return m, nil
}

func (m Model) handleUp() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewEnvironments:
		if m.envIdx > 0 {
			m.envIdx--
		}
	case ViewSecrets:
		if m.secretIdx > 0 {
			m.secretIdx--
		}
	case ViewAuditLog:
		if m.auditIdx > 0 {
			m.auditIdx--
			if m.auditIdx < m.auditScroll {
				m.auditScroll = m.auditIdx
			}
		}
	}
	return m, nil
}

func (m Model) handleDown() (tea.Model, tea.Cmd) {
	switch m.view {
	case ViewEnvironments:
		if m.envIdx < len(m.environments)-1 {
			m.envIdx++
		}
	case ViewSecrets:
		if m.secretIdx < len(m.secrets)-1 {
			m.secretIdx++
		}
	case ViewAuditLog:
		if m.auditIdx < len(m.auditEntries)-1 {
			m.auditIdx++
			maxVisible := m.height - 8
			if maxVisible < 5 {
				maxVisible = 5
			}
			if m.auditIdx >= m.auditScroll+maxVisible {
				m.auditScroll = m.auditIdx - maxVisible + 1
			}
		}
	}
	return m, nil
}

func (m *Model) loadAudit() {
	if m.auditLogger == nil {
		return
	}
	entries, _ := m.auditLogger.Read()
	m.auditEntries = entries
	m.auditIdx = 0
	m.auditScroll = 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
