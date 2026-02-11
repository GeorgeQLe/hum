package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/config"
)

// ScanMode holds state for the scan mode UI.
type ScanMode struct {
	candidates    []config.ScanCandidate
	cursorIdx     int
	selected      map[int]bool
	readmeCache   map[string]string
	readmeScroll  int
	candidateScroll int
	focus         string // "candidates" or "readme"
}

func newScanMode(candidates []config.ScanCandidate) *ScanMode {
	return &ScanMode{
		candidates:  candidates,
		cursorIdx:   0,
		selected:    make(map[int]bool),
		readmeCache: make(map[string]string),
		focus:       "candidates",
	}
}

// scanResultMsg delivers scan results to the model.
type scanResultMsg struct {
	candidates []config.ScanCandidate
}

func (m Model) handleScanKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sm := m.scanMode

	switch {
	case isKey(msg, "tab"):
		if sm.focus == "candidates" {
			sm.focus = "readme"
		} else {
			sm.focus = "candidates"
		}
		return m, nil

	case isKey(msg, "up", "k"):
		if sm.focus == "candidates" {
			if sm.cursorIdx > 0 {
				sm.cursorIdx--
				sm.readmeScroll = 0
				// Scroll candidate list if cursor is above visible area
				if sm.cursorIdx < sm.candidateScroll {
					sm.candidateScroll = sm.cursorIdx
				}
			}
		} else {
			if sm.readmeScroll > 0 {
				sm.readmeScroll--
			}
		}
		return m, nil

	case isKey(msg, "down", "j"):
		if sm.focus == "candidates" {
			if sm.cursorIdx < len(sm.candidates)-1 {
				sm.cursorIdx++
				sm.readmeScroll = 0
				// Scroll candidate list if cursor is below visible area
				visibleRows := m.mainHeight() - 2 // minus header and instructions rows
				if visibleRows > 0 && sm.cursorIdx >= sm.candidateScroll+visibleRows {
					sm.candidateScroll = sm.cursorIdx - visibleRows + 1
				}
			}
		} else {
			sm.readmeScroll++
		}
		return m, nil

	case isKey(msg, " "):
		if sm.selected[sm.cursorIdx] {
			delete(sm.selected, sm.cursorIdx)
		} else {
			sm.selected[sm.cursorIdx] = true
		}
		return m, nil

	case isRune(msg, 'a'):
		if len(sm.selected) == len(sm.candidates) {
			sm.selected = make(map[int]bool)
		} else {
			for i := range sm.candidates {
				sm.selected[i] = true
			}
		}
		return m, nil

	case isKey(msg, "enter"):
		m.exitScanMode(true)
		return m, nil

	case isKey(msg, "esc"):
		m.exitScanMode(false)
		return m, nil

	case isKey(msg, "pgup"):
		if sm.focus == "readme" {
			sm.readmeScroll -= m.logViewHeight() - 1
			if sm.readmeScroll < 0 {
				sm.readmeScroll = 0
			}
		}
		return m, nil

	case isKey(msg, "pgdown"):
		if sm.focus == "readme" {
			sm.readmeScroll += m.logViewHeight() - 1
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) exitScanMode(confirmed bool) {
	sm := m.scanMode
	if confirmed && len(sm.selected) > 0 {
		existingNames := make(map[string]bool)
		for _, a := range m.apps {
			existingNames[a.Name] = true
		}

		var addedNames []string
		for idx := range sm.selected {
			c := sm.candidates[idx]
			name := c.Name

			// Auto-resolve name collisions
			if existingNames[name] {
				suffix := 2
				for existingNames[fmt.Sprintf("%s-%d", c.Name, suffix)] {
					suffix++
				}
				name = fmt.Sprintf("%s-%d", c.Name, suffix)
			}

			app := config.App{
				Name:    name,
				Dir:     c.Dir,
				Command: c.Command,
				Ports:   c.Ports,
			}
			if app.Validate() != nil {
				continue
			}

			m.apps = append(m.apps, app)
			existingNames[name] = true
			addedNames = append(addedNames, name)
		}

		m.scanMode = nil
		if len(addedNames) > 0 {
			m.layoutDirty = true
			m.saveConfig()
			m.systemLog(fmt.Sprintf("Added %d app(s): %s", len(addedNames), strings.Join(addedNames, ", ")))
		} else {
			m.systemLog("No apps were added.")
		}
	} else {
		m.scanMode = nil
		if confirmed {
			m.systemLog("No apps selected.")
		} else {
			m.systemLog("Scan cancelled.")
		}
	}
}

// renderScanCandidateRow renders a row in the scan mode sidebar.
func renderScanCandidateRow(m *Model, rowIdx, width int) string {
	sm := m.scanMode
	if sm == nil {
		return strings.Repeat(" ", width)
	}

	// Row 0: header
	if rowIdx == 0 {
		label := "SCAN RESULTS"
		if sm.focus == "candidates" {
			label = styleBold.Render(label)
		} else {
			label = styleDim.Render(label)
		}
		return padRight(" "+label, width)
	}

	// Row 1: instructions
	if rowIdx == 1 {
		return padRight(styleDim.Render(" Space: toggle | a: all | Enter: confirm"), width)
	}

	idx := sm.candidateScroll + (rowIdx - 2)
	if idx < 0 || idx >= len(sm.candidates) {
		return strings.Repeat(" ", width)
	}

	c := sm.candidates[idx]
	isSelected := sm.cursorIdx == idx

	// Checkbox
	check := "[ ]"
	if sm.selected[idx] {
		check = "[x]"
	}

	prefix := "  "
	if isSelected {
		prefix = "▸ "
	}

	// Truncate name
	maxNameLen := width - 7 // prefix(2) + check(3) + space(1) + padding(1)
	displayName := c.Name
	if len(displayName) > maxNameLen && maxNameLen > 1 {
		displayName = displayName[:maxNameLen-1] + "…"
	}

	text := prefix + check + " " + displayName
	if isSelected && sm.focus == "candidates" {
		return styleInverse.Render(padRight(text, width))
	}
	if isSelected {
		return styleBold.Render(padRight(text, width))
	}
	return padRight(text, width)
}

// renderScanReadmeRow renders a row in the scan mode right pane.
func renderScanReadmeRow(m *Model, rowIdx, width int) string {
	sm := m.scanMode
	if sm == nil {
		return strings.Repeat(" ", width)
	}

	// Row 0: header with candidate details
	if rowIdx == 0 {
		if sm.cursorIdx >= 0 && sm.cursorIdx < len(sm.candidates) {
			c := sm.candidates[sm.cursorIdx]
			header := fmt.Sprintf(" %s  %s",
				styleBold.Render(c.Name),
				styleDim.Render(c.Dir),
			)
			return padRight(header, width)
		}
		return padRight(" "+styleDim.Render("No selection"), width)
	}

	if sm.cursorIdx < 0 || sm.cursorIdx >= len(sm.candidates) {
		return strings.Repeat(" ", width)
	}

	c := sm.candidates[sm.cursorIdx]

	// Build info lines
	var infoLines []string
	infoLines = append(infoLines, fmt.Sprintf(" Command: %s", c.Command))
	ports := make([]string, len(c.Ports))
	for i, p := range c.Ports {
		ports[i] = fmt.Sprintf("%d", p)
	}
	infoLines = append(infoLines, fmt.Sprintf(" Ports:   %s", strings.Join(ports, ", ")))
	infoLines = append(infoLines, fmt.Sprintf(" Script:  %s", c.DevScript))
	infoLines = append(infoLines, "")

	// Load and display README
	readme := m.loadReadme(c.Dir)
	if readme != "" {
		infoLines = append(infoLines, " "+styleBold.Render("README"))
		for _, line := range strings.Split(readme, "\n") {
			infoLines = append(infoLines, " "+line)
		}
	}

	lineIdx := sm.readmeScroll + (rowIdx - 1)
	if lineIdx >= 0 && lineIdx < len(infoLines) {
		return truncateToWidth(infoLines[lineIdx], width)
	}

	return strings.Repeat(" ", width)
}

func (m *Model) loadReadme(dir string) string {
	sm := m.scanMode
	if sm == nil {
		return ""
	}

	if cached, ok := sm.readmeCache[dir]; ok {
		return cached
	}

	fullDir := filepath.Join(m.projectRoot, dir)
	for _, name := range []string{"README.md", "readme.md", "README", "README.txt"} {
		data, err := os.ReadFile(filepath.Join(fullDir, name))
		if err == nil {
			content := string(data)
			// Limit to first 100 lines
			lines := strings.Split(content, "\n")
			if len(lines) > 100 {
				lines = lines[:100]
			}
			result := strings.Join(lines, "\n")
			sm.readmeCache[dir] = result
			return result
		}
	}

	sm.readmeCache[dir] = ""
	return ""
}
