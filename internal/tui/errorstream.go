package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/hum/internal/process"
)

// ErrorStreamMode is an overlay mode that displays a browsable, collapsible
// error stream in place of the normal log view.
type ErrorStreamMode struct {
	cursor   int
	scroll   int
	expanded map[int]bool
	follow   bool
	allApps  bool // true when system log is selected (show all app errors)
}

func newErrorStreamMode(allApps bool) *ErrorStreamMode {
	return &ErrorStreamMode{
		expanded: make(map[int]bool),
		follow:   true,
		allApps:  allApps,
	}
}

// errorStreamEntry holds rendering data for one error in the stream.
type errorStreamEntry struct {
	group   process.ErrorGroup
	appName string // only used in all-apps view
}

// buildErrorEntries collects error groups for display.
func (m *Model) buildErrorEntries() []errorStreamEntry {
	if m.errorStream.allApps {
		// Collect errors from all apps, sorted chronologically
		var entries []errorStreamEntry
		for _, app := range m.apps {
			buf := m.procManager.GetErrorBuffer(app.Name)
			groups := buf.GroupedErrors()
			for _, g := range groups {
				entries = append(entries, errorStreamEntry{
					group:   g,
					appName: app.Name,
				})
			}
		}
		return entries
	}

	// Single app mode
	app := m.getSelectedApp()
	if app == nil {
		return nil
	}
	buf := m.procManager.GetErrorBuffer(app.Name)
	groups := buf.GroupedErrors()
	entries := make([]errorStreamEntry, len(groups))
	for i, g := range groups {
		entries[i] = errorStreamEntry{group: g, appName: app.Name}
	}
	return entries
}

// renderErrorStreamHeader renders the error stream header line.
func (m *Model) renderErrorStreamHeader(width int) string {
	entries := m.cachedErrorEntries
	count := len(entries)

	var header string
	if m.errorStream.allApps {
		header = fmt.Sprintf(" %s  %s",
			styleBold.Render(fmt.Sprintf("ERRORS (%d)", count)),
			styleDim.Render("all apps"),
		)
	} else {
		appName := ""
		if app := m.getSelectedApp(); app != nil {
			appName = app.Name
		}
		header = fmt.Sprintf(" %s  %s",
			styleBold.Render(fmt.Sprintf("ERRORS (%d)", count)),
			styleDim.Render(appName),
		)
	}
	return padRight(header, width)
}

// renderErrorStreamRow renders a single row of the error stream overlay.
// Row 0 is the header; subsequent rows are error entries.
func renderErrorStreamRow(m *Model, rowIdx, width int) string {
	if m.errorStream == nil {
		return renderLogRow(m, rowIdx, width)
	}

	// Row 0: header
	if rowIdx == 0 {
		return m.renderErrorStreamHeader(width)
	}

	entries := m.cachedErrorEntries
	if len(entries) == 0 {
		if rowIdx == 1 {
			return padRight(" "+styleDim.Render("No errors detected"), width)
		}
		return strings.Repeat(" ", width)
	}

	// Build visual lines: each entry is 1 collapsed line or N expanded lines
	type visLine struct {
		text    string
		entryIdx int
	}
	var vlines []visLine

	for i, entry := range entries {
		isExpanded := m.errorStream.expanded[i]
		isSelected := m.errorStream.cursor == i

		if isExpanded {
			// Expanded header
			vlines = append(vlines, visLine{
				text:     formatExpandedHeader(entry, isSelected, width, m.errorStream.allApps),
				entryIdx: i,
			})
			// Expanded body lines
			bodyLines := getErrorBodyLines(entry)
			for _, bl := range bodyLines {
				vlines = append(vlines, visLine{
					text:     formatBodyLine(bl, width),
					entryIdx: i,
				})
			}
		} else {
			// Collapsed single line
			vlines = append(vlines, visLine{
				text:     formatCollapsedLine(entry, isSelected, width, m.errorStream.allApps),
				entryIdx: i,
			})
		}
	}

	// Apply scroll offset
	vlIdx := rowIdx - 1 + m.errorStream.scroll
	if vlIdx < 0 || vlIdx >= len(vlines) {
		return strings.Repeat(" ", width)
	}

	return vlines[vlIdx].text
}

func formatCollapsedLine(entry errorStreamEntry, isSelected bool, width int, showApp bool) string {
	g := entry.group
	prefix := "  "
	arrow := "▸"
	if isSelected {
		prefix = " "
		arrow = "▸"
	}

	// Error type and message
	errType := ""
	msg := ""
	if g.Latest != nil && g.Latest.Parsed != nil {
		errType = g.Latest.Parsed.ErrorType
		msg = g.Latest.Parsed.Message
	} else if g.Latest != nil && len(g.Latest.Lines) > 0 {
		msg = process.StripAnsi(g.Latest.Lines[0])
	}

	// Location
	loc := ""
	if g.Latest != nil && g.Latest.Parsed != nil && g.Latest.Parsed.Location != nil {
		loc = g.Latest.Parsed.Location.String()
	}

	// Timestamp
	ts := g.LastSeen.Format("15:04:05")

	// Count badge
	countBadge := ""
	if g.Count > 1 {
		countBadge = fmt.Sprintf("x%d", g.Count)
	}

	// App name for all-apps view
	appTag := ""
	if showApp {
		appTag = styleDim.Render("["+entry.appName+"]") + " "
	}

	// Build the line
	var line strings.Builder
	line.WriteString(prefix)

	if isSelected {
		line.WriteString(styleError.Render(arrow))
	} else {
		line.WriteString(styleDim.Render(arrow))
	}
	line.WriteString(" ")

	line.WriteString(appTag)

	if errType != "" {
		line.WriteString(styleError.Render(errType))
		line.WriteString(styleDim.Render(": "))
	}

	// Truncate message to fit
	remaining := width - visLen(line.String()) - stringWidth(ts) - stringWidth(countBadge) - stringWidth(loc) - 6
	if remaining < 10 {
		remaining = 10
	}
	displayMsg := msg
	if stringWidth(displayMsg) > remaining {
		displayMsg = truncateString(displayMsg, remaining-1) + "…"
	}
	line.WriteString(displayMsg)

	// Right-aligned metadata
	suffix := ""
	if loc != "" {
		suffix += "  " + styleDim.Render(loc)
	}
	suffix += "  " + styleDim.Render(ts)
	if countBadge != "" {
		suffix += "  " + styleErrorCount.Render(countBadge)
	}

	content := line.String() + suffix

	if isSelected {
		return truncateToWidth(styleInverse.Render(padRight(content, width)), width)
	}
	return truncateToWidth(content, width)
}

func formatExpandedHeader(entry errorStreamEntry, isSelected bool, width int, showApp bool) string {
	g := entry.group

	arrow := "▾"
	prefix := "  "
	if isSelected {
		prefix = " "
	}

	errType := ""
	msg := ""
	if g.Latest != nil && g.Latest.Parsed != nil {
		errType = g.Latest.Parsed.ErrorType
		msg = g.Latest.Parsed.Message
	} else if g.Latest != nil && len(g.Latest.Lines) > 0 {
		msg = process.StripAnsi(g.Latest.Lines[0])
	}

	loc := ""
	if g.Latest != nil && g.Latest.Parsed != nil && g.Latest.Parsed.Location != nil {
		loc = g.Latest.Parsed.Location.String()
	}

	ts := g.LastSeen.Format("15:04:05")
	countBadge := ""
	if g.Count > 1 {
		countBadge = fmt.Sprintf("x%d", g.Count)
	}

	appTag := ""
	if showApp {
		appTag = styleDim.Render("["+entry.appName+"]") + " "
	}

	var line strings.Builder
	line.WriteString(prefix)
	if isSelected {
		line.WriteString(styleError.Render(arrow))
	} else {
		line.WriteString(styleDim.Render(arrow))
	}
	line.WriteString(" ")
	line.WriteString(appTag)

	if errType != "" {
		line.WriteString(styleBold.Render(errType))
		line.WriteString(": ")
	}
	line.WriteString(msg)

	suffix := ""
	if loc != "" {
		suffix += "  " + styleDim.Render(loc)
	}
	suffix += "  " + styleDim.Render(ts)
	if countBadge != "" {
		suffix += "  " + styleErrorCount.Render(countBadge)
	}

	content := line.String() + suffix

	if isSelected {
		return truncateToWidth(styleInverse.Render(padRight(content, width)), width)
	}
	return truncateToWidth(padRight(content, width), width)
}

func formatBodyLine(line string, width int) string {
	return truncateToWidth("     "+line, width)
}

// getErrorBodyLines returns the detail lines for an expanded error.
func getErrorBodyLines(entry errorStreamEntry) []string {
	if entry.group.Latest == nil {
		return nil
	}

	ce := entry.group.Latest
	if ce.Parsed != nil {
		var lines []string
		// Main error message
		if ce.Parsed.ErrorType != "" {
			lines = append(lines, ce.Parsed.ErrorType+": "+ce.Parsed.Message)
		} else {
			lines = append(lines, ce.Parsed.Message)
		}
		// Stack trace
		lines = append(lines, ce.Parsed.StackTrace...)
		return lines
	}

	// Fallback: show raw lines (ANSI stripped)
	lines := make([]string, len(ce.Lines))
	for i, l := range ce.Lines {
		lines[i] = process.StripAnsi(l)
	}
	return lines
}

// handleErrorStreamKeypress handles key events in error stream mode.
func (m Model) handleErrorStreamKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	es := m.errorStream
	entries := m.buildErrorEntries()
	maxIdx := len(entries) - 1
	if maxIdx < 0 {
		maxIdx = 0
	}

	switch {
	case isRune(msg, 'x') || isKey(msg, "esc"):
		// Exit error stream
		m.errorStream = nil
		return m, nil

	case isKey(msg, "enter"):
		// Toggle expand/collapse
		if len(entries) > 0 && es.cursor >= 0 && es.cursor < len(entries) {
			es.expanded[es.cursor] = !es.expanded[es.cursor]
		}
		return m, nil

	case isKey(msg, "up", "k"):
		if es.cursor > 0 {
			es.cursor--
			if es.cursor < es.scroll {
				es.scroll = es.cursor
			}
		}
		return m, nil

	case isKey(msg, "down", "j"):
		if es.cursor < maxIdx {
			es.cursor++
			viewH := m.logViewHeight()
			if es.cursor >= es.scroll+viewH {
				es.scroll = es.cursor - viewH + 1
			}
		}
		return m, nil

	case isRune(msg, 'e'):
		// Copy full error block
		return m.copyFullError(entries)

	case isRune(msg, 'm'):
		// Copy error message only
		return m.copyErrorMessage(entries)

	case isRune(msg, 'l'):
		// Copy source location
		return m.copyErrorLocation(entries)

	case isRune(msg, 'c'):
		// Clear all errors for current app
		m.clearStreamErrors()
		return m, nil

	case isKey(msg, "pgup"):
		es.scroll -= m.logViewHeight() - 1
		if es.scroll < 0 {
			es.scroll = 0
		}
		return m, nil

	case isKey(msg, "pgdown"):
		es.scroll += m.logViewHeight() - 1
		// Clamp to max scroll position
		totalVisualLines := 0
		for i, entry := range entries {
			if es.expanded[i] {
				totalVisualLines += 1 + len(getErrorBodyLines(entry))
			} else {
				totalVisualLines++
			}
		}
		viewHeight := m.logViewHeight()
		maxScroll := totalVisualLines - viewHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if es.scroll > maxScroll {
			es.scroll = maxScroll
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) copyFullError(entries []errorStreamEntry) (Model, tea.Cmd) {
	if m.errorStream == nil || m.errorStream.cursor < 0 || m.errorStream.cursor >= len(entries) {
		return *m, nil
	}
	entry := entries[m.errorStream.cursor]
	if entry.group.Latest == nil {
		return *m, nil
	}

	var text string
	if entry.group.Latest.Parsed != nil {
		pe := entry.group.Latest.Parsed
		text = strings.Join(pe.PlainLines, "\n")
	} else {
		lines := make([]string, len(entry.group.Latest.Lines))
		for i, l := range entry.group.Latest.Lines {
			lines[i] = process.StripAnsi(l)
		}
		text = strings.Join(lines, "\n")
	}

	if err := process.CopyToClipboard(text); err != nil {
		m.systemLog(fmt.Sprintf("Clipboard error: %s", err))
	} else {
		m.systemLog("Error copied to clipboard")
	}
	return *m, nil
}

func (m *Model) copyErrorMessage(entries []errorStreamEntry) (Model, tea.Cmd) {
	if m.errorStream == nil || m.errorStream.cursor < 0 || m.errorStream.cursor >= len(entries) {
		return *m, nil
	}
	entry := entries[m.errorStream.cursor]
	if entry.group.Latest == nil || entry.group.Latest.Parsed == nil {
		m.systemLog("No structured message available")
		return *m, nil
	}

	msg := entry.group.Latest.Parsed.Message
	if entry.group.Latest.Parsed.ErrorType != "" {
		msg = entry.group.Latest.Parsed.ErrorType + ": " + msg
	}

	if err := process.CopyToClipboard(msg); err != nil {
		m.systemLog(fmt.Sprintf("Clipboard error: %s", err))
	} else {
		m.systemLog("Error message copied to clipboard")
	}
	return *m, nil
}

func (m *Model) copyErrorLocation(entries []errorStreamEntry) (Model, tea.Cmd) {
	if m.errorStream == nil || m.errorStream.cursor < 0 || m.errorStream.cursor >= len(entries) {
		return *m, nil
	}
	entry := entries[m.errorStream.cursor]
	if entry.group.Latest == nil || entry.group.Latest.Parsed == nil || entry.group.Latest.Parsed.Location == nil {
		m.systemLog("No source location available")
		return *m, nil
	}

	loc := entry.group.Latest.Parsed.Location.String()
	if err := process.CopyToClipboard(loc); err != nil {
		m.systemLog(fmt.Sprintf("Clipboard error: %s", err))
	} else {
		m.systemLog(fmt.Sprintf("Location copied: %s", loc))
	}
	return *m, nil
}

func (m *Model) clearStreamErrors() {
	if m.errorStream.allApps {
		m.procManager.ClearAllErrors()
		m.systemLog("All errors cleared")
	} else if app := m.getSelectedApp(); app != nil {
		m.procManager.ClearErrors(app.Name)
		m.systemLog("Errors cleared for " + app.Name)
	}
	// Reset cursor
	m.errorStream.cursor = 0
	m.errorStream.scroll = 0
	m.errorStream.expanded = make(map[int]bool)
}
