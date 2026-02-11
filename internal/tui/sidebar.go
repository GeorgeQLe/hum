package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/georgele/devctl/internal/health"
	"github.com/georgele/devctl/internal/process"
)

const systemName = "devctl"

// sidebarRow represents a row in the sidebar, which may be a group header or an app entry.
type sidebarRow struct {
	isGroupHeader bool
	groupName     string
	appIdx        int // index into m.apps (-1 for system/group headers)
	selectIdx     int // index for selection tracking
}

// buildSidebarRows computes the sidebar row layout with group headers and pinned sorting.
func buildSidebarRows(m *Model) []sidebarRow {
	var rows []sidebarRow

	// System entry
	rows = append(rows, sidebarRow{appIdx: -1, selectIdx: 0})

	// Sort apps: pinned first, then by group, then original order
	type indexedApp struct {
		origIdx int
		pinned  bool
		group   string
	}
	ordered := make([]indexedApp, len(m.apps))
	for i, a := range m.apps {
		pinned := a.Pinned != nil && *a.Pinned
		ordered[i] = indexedApp{origIdx: i, pinned: pinned, group: a.Group}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].pinned != ordered[j].pinned {
			return ordered[i].pinned
		}
		if ordered[i].group != ordered[j].group {
			return ordered[i].group < ordered[j].group
		}
		return false
	})

	lastGroup := ""
	for _, o := range ordered {
		g := m.apps[o.origIdx].Group
		if g != "" && g != lastGroup {
			rows = append(rows, sidebarRow{isGroupHeader: true, groupName: g, appIdx: -1, selectIdx: -1})
			lastGroup = g
		} else if g == "" && lastGroup != "" {
			lastGroup = ""
		}
		rows = append(rows, sidebarRow{appIdx: o.origIdx, selectIdx: o.origIdx + 1})
	}
	return rows
}

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

	sidebarRows := buildSidebarRows(m)
	dataIdx := rowIdx - 1 // -1 for header
	if dataIdx < 0 || dataIdx >= len(sidebarRows) {
		return strings.Repeat(" ", width)
	}

	row := sidebarRows[dataIdx]
	if row.isGroupHeader {
		label := styleDim.Render(" ─ " + row.groupName + " ")
		return padRight(label, width)
	}
	if row.appIdx == -1 {
		// System entry
		return renderSidebarEntry(m, 0, systemName, "", width)
	}

	app := m.apps[row.appIdx]
	status := m.procManager.GetStatus(app.Name)
	return renderSidebarAppEntry(m, row.selectIdx, app.Name, status, width)
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

	// Health indicator
	healthSuffix := ""
	healthSuffixLen := 0
	if m.healthChecker != nil && m.healthChecker.HasCheck(name) {
		hs := m.healthChecker.GetStatus(name)
		switch hs {
		case health.StatusHealthy:
			healthSuffix = styleStatusRunning.Render("♥")
			healthSuffixLen = 1
		case health.StatusUnhealthy:
			healthSuffix = styleStatusCrashed.Render("♥")
			healthSuffixLen = 1
		}
	}

	// Resource threshold indicator
	resourceSuffix := ""
	resourceSuffixLen := 0
	if m.resourceMonitor != nil && m.resourceMonitor.IsExceeded(name) {
		resourceSuffix = styleStatusCrashed.Render("▲")
		resourceSuffixLen = 1
	}

	// Truncate name if needed (width - 3 prefix - 2 dot+space - error indicator - health - resource)
	maxNameLen := width - 5 - errorSuffixLen - healthSuffixLen - resourceSuffixLen
	displayName := name
	if len(displayName) > maxNameLen && maxNameLen > 1 {
		displayName = displayName[:maxNameLen-1] + "…"
	}

	padLen := width - 3 - len(displayName) - 2 - errorSuffixLen - healthSuffixLen - resourceSuffixLen
	if padLen < 0 {
		padLen = 0
	}
	padding := strings.Repeat(" ", padLen)

	dot := dotStyle(dotChar)
	suffixes := resourceSuffix + healthSuffix + errorSuffix + dot

	if isSelected && m.focusArea == focusSidebar {
		return styleInverse.Render(fmt.Sprintf("%s%s%s ", prefix, displayName, padding)) + suffixes
	}
	if isSelected {
		return styleBold.Render(prefix+displayName) + padding + " " + suffixes
	}
	return prefix + displayName + padding + " " + suffixes
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
