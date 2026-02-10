package tui

import (
	"fmt"
	"strings"

	"github.com/georgele/devctl/internal/process"
)

const systemName = "devctl"

// renderSidebar renders the sidebar content for the given row.
func renderSidebar(m *Model, rowIdx, width int) string {
	// Row 0: header
	if rowIdx == 0 {
		label := "APPS"
		if m.focusArea == focusSidebar {
			label = styleBold.Render(label)
		} else {
			label = styleDim.Render(label)
		}
		return padRight(" "+label, width)
	}

	// Row 1: devctl system entry
	if rowIdx == 1 {
		return renderSidebarEntry(m, 0, systemName, "", width)
	}

	// Row 2+: apps
	appIdx := rowIdx - 2
	if appIdx < 0 || appIdx >= len(m.apps) {
		return strings.Repeat(" ", width)
	}

	app := m.apps[appIdx]
	status := m.procManager.GetStatus(app.Name)
	return renderSidebarAppEntry(m, appIdx+1, app.Name, status, width)
}

func renderSidebarEntry(m *Model, selectIdx int, name string, _ string, width int) string {
	isSelected := m.selectedIdx == selectIdx
	prefix := "   "
	if isSelected {
		prefix = " ▸ "
	}

	padLen := width - 3 - visLen(name)
	if padLen < 0 {
		padLen = 0
	}
	padding := strings.Repeat(" ", padLen)

	if isSelected && m.focusArea == focusSidebar {
		return styleInverse.Render(prefix + name + padding)
	}
	if isSelected {
		return styleBold.Render(prefix+name) + padding
	}
	return styleDim.Render(prefix+name) + padding
}

func renderSidebarAppEntry(m *Model, selectIdx int, name string, status process.Status, width int) string {
	isSelected := m.selectedIdx == selectIdx

	// Status dot
	var dotChar string
	var dotStyle func(string) string
	switch status {
	case process.StatusRunning:
		dotChar = "●"
		dotStyle = func(s string) string { return styleStatusRunning.Render(s) }
	case process.StatusCrashed:
		dotChar = "●"
		dotStyle = func(s string) string { return styleStatusCrashed.Render(s) }
	case process.StatusStopping:
		dotChar = "●"
		dotStyle = func(s string) string { return styleStatusStopping.Render(s) }
	default:
		dotChar = "○"
		dotStyle = func(s string) string { return styleDim.Render(s) }
	}

	prefix := "   "
	if isSelected {
		prefix = " ▸ "
	}

	// Error indicator
	errorCount := m.procManager.GetErrorCount(name)
	hasError := errorCount > 0
	errorSuffix := ""
	errorSuffixLen := 0
	if hasError {
		errorSuffix = styleStatusCrashed.Render("!")
		errorSuffixLen = 1
	}

	// Truncate name if needed (width - 3 prefix - 2 dot+space - error indicator)
	maxNameLen := width - 5 - errorSuffixLen
	displayName := name
	if len(displayName) > maxNameLen && maxNameLen > 1 {
		displayName = displayName[:maxNameLen-1] + "…"
	}

	padLen := width - 3 - len(displayName) - 2 - errorSuffixLen
	if padLen < 0 {
		padLen = 0
	}
	padding := strings.Repeat(" ", padLen)

	dot := dotStyle(dotChar)

	if isSelected && m.focusArea == focusSidebar {
		return styleInverse.Render(fmt.Sprintf("%s%s%s ", prefix, displayName, padding)) + errorSuffix + dot
	}
	if isSelected {
		return styleBold.Render(prefix+displayName) + padding + " " + errorSuffix + dot
	}
	return prefix + displayName + padding + " " + errorSuffix + dot
}

func visLen(s string) int {
	return len(process.StripAnsi(s))
}

func padRight(s string, width int) string {
	vl := visLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}
