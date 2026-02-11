package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/devctl/internal/process"
)

// TopMode holds state for the live resource dashboard overlay.
type TopMode struct {
	cursor    int
	scroll    int
	sortCol   topSortCol
	sortDesc  bool
}

type topSortCol int

const (
	topSortName topSortCol = iota
	topSortCPU
	topSortMem
	topSortUptime
)

func newTopMode() *TopMode {
	return &TopMode{
		sortCol:  topSortCPU,
		sortDesc: true,
	}
}

type topAppRow struct {
	name       string
	cpu        float64
	memBytes   int64
	uptime     string
	uptimeSecs int
	status     process.Status
}

func (m *Model) buildTopRows() []topAppRow {
	var rows []topAppRow
	for _, app := range m.apps {
		status := m.procManager.GetStatus(app.Name)
		row := topAppRow{
			name:   app.Name,
			status: status,
		}

		if status == process.StatusRunning {
			if latest := m.resourceMonitor.GetLatest(app.Name); latest != nil {
				row.cpu = latest.CPUPercent
				row.memBytes = latest.MemoryRSS
			}
			uptime := m.procManager.Uptime(app.Name)
			row.uptime = formatUptime(uptime)
			row.uptimeSecs = int(uptime.Seconds())
		} else {
			row.uptime = "-"
		}

		rows = append(rows, row)
	}

	// Sort
	tm := m.topMode
	sort.SliceStable(rows, func(i, j int) bool {
		var less bool
		switch tm.sortCol {
		case topSortName:
			less = rows[i].name < rows[j].name
		case topSortCPU:
			less = rows[i].cpu < rows[j].cpu
		case topSortMem:
			less = rows[i].memBytes < rows[j].memBytes
		case topSortUptime:
			less = rows[i].uptimeSecs < rows[j].uptimeSecs
		}
		if tm.sortDesc {
			return !less
		}
		return less
	})

	return rows
}

// renderTopLeftRow renders a row of the left panel (app table).
func renderTopLeftRow(m *Model, rowIdx, width int) string {
	tm := m.topMode
	if tm == nil {
		return strings.Repeat(" ", width)
	}

	// Row 0: title
	if rowIdx == 0 {
		title := " " + styleBold.Render("RESOURCES") + " " + styleDim.Render("(live)")
		return padRight(title, width)
	}

	// Row 1: column headers
	if rowIdx == 1 {
		nameW := width - 30
		if nameW < 8 {
			nameW = 8
		}
		headers := fmt.Sprintf(" %-*s %6s %8s %8s", nameW, "NAME", "CPU", "MEM", "UPTIME")
		return padRight(styleDim.Render(headers), width)
	}

	rows := m.buildTopRows()
	idx := tm.scroll + (rowIdx - 2)
	if idx < 0 || idx >= len(rows) {
		return strings.Repeat(" ", width)
	}

	row := rows[idx]
	isSelected := tm.cursor == idx

	nameW := width - 30
	if nameW < 8 {
		nameW = 8
	}

	// Format values
	cpuStr := "-"
	memStr := "-"
	if row.status == process.StatusRunning {
		cpuStr = fmt.Sprintf("%.1f%%", row.cpu)
		memStr = process.FormatMemory(row.memBytes)
	}

	prefix := "  "
	if isSelected {
		prefix = " \u25b8"
	}

	name := row.name
	if len(name) > nameW {
		name = name[:nameW-1] + "\u2026"
	}

	line := fmt.Sprintf("%s%-*s %6s %8s %8s", prefix, nameW, name, cpuStr, memStr, row.uptime)

	if isSelected {
		return styleInverse.Render(padRight(line, width))
	}
	return padRight(line, width)
}

// renderTopRightRow renders a row of the right panel (detail stats for selected app).
func renderTopRightRow(m *Model, rowIdx, width int) string {
	tm := m.topMode
	if tm == nil {
		return strings.Repeat(" ", width)
	}

	rows := m.buildTopRows()
	if tm.cursor < 0 || tm.cursor >= len(rows) {
		return strings.Repeat(" ", width)
	}

	selected := rows[tm.cursor]
	stats := m.resourceMonitor.GetStats(selected.name)

	var lines []string

	// Header
	lines = append(lines, " "+styleBold.Render(selected.name)+" \u2014 "+styleDim.Render("Stats"))
	lines = append(lines, "")

	if stats != nil && stats.SampleCount > 0 {
		lines = append(lines, fmt.Sprintf(" CPU:  %.1f%% (avg: %.1f%%, peak: %.1f%%)",
			stats.Current.CPUPercent, stats.AvgCPU, stats.MaxCPU))
		lines = append(lines, fmt.Sprintf(" MEM:  %s (avg: %s, peak: %s)",
			process.FormatMemory(stats.Current.MemoryRSS),
			process.FormatMemory(stats.AvgMemory),
			process.FormatMemory(stats.MaxMemory)))
		lines = append(lines, fmt.Sprintf(" Uptime: %s", selected.uptime))
		lines = append(lines, fmt.Sprintf(" Samples: %d (%s)", stats.SampleCount, formatUptime(stats.Duration)))

		// Threshold info
		if m.resourceMonitor.IsExceeded(selected.name) {
			lines = append(lines, "")
			lines = append(lines, " "+styleStatusCrashed.Render("\u25b2 THRESHOLD EXCEEDED"))
		}
	} else {
		if selected.status == process.StatusRunning {
			lines = append(lines, " Collecting samples...")
		} else {
			lines = append(lines, " "+styleDim.Render("Not running"))
		}
	}

	lines = append(lines, "")
	lines = append(lines, styleDim.Render(" c:CPU  m:MEM  n:name  u:uptime  r:reverse  Esc:exit"))

	if rowIdx >= 0 && rowIdx < len(lines) {
		return truncateToWidth(lines[rowIdx], width)
	}
	return strings.Repeat(" ", width)
}

func (m Model) handleTopKeypress(msg tea.KeyMsg) (Model, tea.Cmd) {
	tm := m.topMode

	switch {
	case isKey(msg, "esc", "q"):
		m.topMode = nil
		return m, nil

	case isKey(msg, "up", "k"):
		if tm.cursor > 0 {
			tm.cursor--
			if tm.cursor < tm.scroll {
				tm.scroll = tm.cursor
			}
		}
		return m, nil

	case isKey(msg, "down", "j"):
		maxIdx := len(m.apps) - 1
		if tm.cursor < maxIdx {
			tm.cursor++
			viewH := m.mainHeight() - 2 // minus title and headers
			if tm.cursor >= tm.scroll+viewH {
				tm.scroll = tm.cursor - viewH + 1
			}
		}
		return m, nil

	case isRune(msg, 'c'):
		tm.sortCol = topSortCPU
		tm.sortDesc = true
		return m, nil

	case isRune(msg, 'm'):
		tm.sortCol = topSortMem
		tm.sortDesc = true
		return m, nil

	case isRune(msg, 'n'):
		tm.sortCol = topSortName
		tm.sortDesc = false
		return m, nil

	case isRune(msg, 'u'):
		tm.sortCol = topSortUptime
		tm.sortDesc = true
		return m, nil

	case isRune(msg, 'r'):
		tm.sortDesc = !tm.sortDesc
		return m, nil
	}

	return m, nil
}
