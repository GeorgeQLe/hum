package tui

import (
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// FilterMode holds state for log line filtering.
type FilterMode struct {
	pattern string
	regex   *regexp.Regexp
}

// newFilterMode creates a new filter mode.
func newFilterMode() *FilterMode {
	return &FilterMode{}
}

// compile compiles the filter pattern into a regex.
func (fm *FilterMode) compile() {
	if fm.pattern == "" {
		fm.regex = nil
		return
	}
	re, err := regexp.Compile("(?i)" + fm.pattern)
	if err != nil {
		escaped := regexp.QuoteMeta(fm.pattern)
		re, _ = regexp.Compile("(?i)" + escaped)
	}
	fm.regex = re
}

// matches returns true if the line matches the filter pattern.
func (fm *FilterMode) matches(line string) bool {
	if fm.regex == nil {
		return true
	}
	return fm.regex.MatchString(line)
}

// handleFilterKeypress processes key events in filter mode.
func (m Model) handleFilterKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isKey(msg, "esc", "enter"):
		if msg.String() == "esc" {
			m.filterMode = nil
		}
		// On enter, keep filter active but exit editing
		return m, nil

	case isKey(msg, "backspace"):
		if len(m.filterMode.pattern) > 0 {
			m.filterMode.pattern = m.filterMode.pattern[:len(m.filterMode.pattern)-1]
			m.filterMode.compile()
		}
		return m, nil

	case isCtrl(msg, "u"):
		m.filterMode.pattern = ""
		m.filterMode.compile()
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.filterMode.pattern += string(msg.Runes)
		m.filterMode.compile()
		return m, nil
	}

	return m, nil
}

// isLineVisible checks whether a line passes the active filter.
func (m *Model) isLineVisible(text string) bool {
	if m.filterMode == nil || m.filterMode.regex == nil {
		return true
	}
	stripped := strings.TrimSpace(text)
	return m.filterMode.matches(stripped)
}
