package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/georgele/devctl/internal/process"
)

// renderLogRow renders a single row of the log pane.
func renderLogRow(m *Model, rowIdx, width int) string {
	app := m.getSelectedApp()

	// Row 0: header
	if rowIdx == 0 {
		if m.selectedIdx == 0 {
			header := " " + styleBold.Render("devctl") + "  " + styleDim.Render("system log")
			return padRight(header, width)
		}
		if app == nil {
			return padRight(" "+styleDim.Render("No apps configured"), width)
		}
		status := m.procManager.GetStatus(app.Name)
		dot, statusStyle := statusIndicator(status)
		header := fmt.Sprintf(" %s  %s %s",
			styleBold.Render(app.Name),
			statusStyle(dot),
			statusStyle(string(status)),
		)
		// Append error count if any
		if errCount := m.procManager.GetErrorCount(app.Name); errCount > 0 {
			errStr := fmt.Sprintf("%d error", errCount)
			if errCount > 1 {
				errStr += "s"
			}
			header += "  " + styleError.Render(errStr)
		}
		return padRight(header, width)
	}

	// Log content
	bufName := m.getSelectedBufName()
	logBuf := m.procManager.GetLogBuffer(bufName)
	lineIdx := logBuf.ScrollPos + (rowIdx - 1)

	if line, ok := logBuf.GetLine(lineIdx); ok {
		text := line.Text

		// Apply search highlighting
		if m.searchMode != nil && m.searchMode.pattern != "" {
			text = m.highlightSearch(text, lineIdx)
		}

		// Prepend timestamp if enabled
		if m.showTimestamps {
			ts := formatTimestamp(line.Timestamp)
			text = styleDim.Render(ts) + " " + text
		}

		// Dim stderr lines but preserve color info
		if line.IsStderr {
			text = styleDim.Render(text)
		}

		return truncateToWidth(" "+text, width)
	}

	return strings.Repeat(" ", width)
}

func statusIndicator(status process.Status) (string, func(string) string) {
	switch status {
	case process.StatusRunning:
		return "●", func(s string) string { return styleStatusRunning.Render(s) }
	case process.StatusCrashed:
		return "●", func(s string) string { return styleStatusCrashed.Render(s) }
	case process.StatusStopping:
		return "●", func(s string) string { return styleStatusStopping.Render(s) }
	default:
		return "○", func(s string) string { return styleDim.Render(s) }
	}
}

func formatTimestamp(t time.Time) string {
	return t.Format("15:04:05")
}

func formatUptime(d time.Duration) string {
	secs := int(d.Seconds())
	mins := secs / 60
	hrs := mins / 60
	if hrs > 0 {
		return fmt.Sprintf("%dh %dm", hrs, mins%60)
	}
	if mins > 0 {
		return fmt.Sprintf("%dm %ds", mins, secs%60)
	}
	return fmt.Sprintf("%ds", secs)
}

func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	stripped := process.StripAnsi(s)
	if len(stripped) <= width {
		return s + strings.Repeat(" ", width-len(stripped))
	}
	// Truncate by visual width while preserving ANSI escape sequences
	var result strings.Builder
	visualPos := 0
	i := 0
	for i < len(s) && visualPos < width {
		// Check for ANSI escape sequence
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Find end of ANSI sequence
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++ // include the terminating letter
			}
			result.WriteString(s[i:j])
			i = j
			continue
		}
		result.WriteByte(s[i])
		visualPos++
		i++
	}
	// Reset any active ANSI styling
	result.WriteString("\x1b[0m")
	return result.String()
}
