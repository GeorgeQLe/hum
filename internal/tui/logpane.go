package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/georgele/hum/internal/process"
	"github.com/mattn/go-runewidth"
)

// visualLine represents a single rendered line in the log pane,
// which may be one of several wrapped segments from a single LogLine.
type visualLine struct {
	text      string
	lineIdx   int // original log line index (for search highlighting)
	isStderr  bool
	timestamp time.Time
}

// computeVisualLines pre-computes the wrapped lines visible in the current view.
// If filter is non-nil, only lines matching the filter are included.
func computeVisualLines(logBuf *process.LogBuffer, scrollPos, viewHeight, contentWidth int, filter *FilterMode) []visualLine {
	lines, _, _ := logBuf.Snapshot()
	var result []visualLine

	for i := scrollPos; i < len(lines) && len(result) < viewHeight; i++ {
		// Apply filter if active
		if filter != nil && filter.regex != nil {
			stripped := process.StripAnsi(lines[i].Text)
			if !filter.regex.MatchString(stripped) {
				continue
			}
		}

		wrapped := wrapLine(lines[i].Text, contentWidth)
		if len(wrapped) == 0 {
			wrapped = []string{""}
		}
		for _, w := range wrapped {
			if len(result) >= viewHeight {
				break
			}
			result = append(result, visualLine{
				text:      w,
				lineIdx:   i,
				isStderr:  lines[i].IsStderr,
				timestamp: lines[i].Timestamp,
			})
		}
	}
	return result
}

// wrapLine word-wraps a line to fit within maxWidth visual columns (B1).
// Preserves ANSI escape sequences. Returns at least one line.
func wrapLine(s string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{s}
	}

	stripped := process.StripAnsi(s)
	if stringWidth(stripped) <= maxWidth {
		return []string{s}
	}

	var lines []string
	var current strings.Builder
	visualPos := 0
	i := 0

	for i < len(s) {
		// Pass through ANSI escape sequences without counting width
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			current.WriteString(s[i:j])
			i = j
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)

		if visualPos+rw > maxWidth {
			// Reset ANSI at end of wrapped line and start new line
			current.WriteString("\x1b[0m")
			lines = append(lines, current.String())
			current.Reset()
			visualPos = 0
		}

		current.WriteString(s[i : i+size])
		visualPos += rw
		i += size
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	if len(lines) == 0 {
		lines = []string{""}
	}

	return lines
}

// renderLogRow renders a single row of the log pane.
// Uses pre-computed visual lines from computeVisualLines when available.
func renderLogRow(m *Model, rowIdx, width int) string {
	app := m.getSelectedApp()

	// Row 0: header
	if rowIdx == 0 {
		if m.selectedIdx == 0 {
			header := " " + styleBold.Render("humrun") + "  " + styleDim.Render("system log")
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

	// Use pre-computed visual lines if available
	vlIdx := rowIdx - 1
	if vlIdx < len(m.visibleLines) {
		vl := m.visibleLines[vlIdx]
		text := vl.text

		// Apply search highlighting
		if m.searchMode != nil && m.searchMode.pattern != "" {
			text = m.highlightSearch(text, vl.lineIdx)
		}

		// Prepend timestamp if enabled (only for first wrapped line of a log entry)
		if m.showTimestamps {
			isFirst := vlIdx == 0 || m.visibleLines[vlIdx-1].lineIdx != vl.lineIdx
			if isFirst {
				ts := formatTimestamp(vl.timestamp)
				text = styleDim.Render(ts) + " " + text
			} else {
				text = strings.Repeat(" ", 9) + text // align with timestamp
			}
		}

		// Dim stderr lines but preserve color info
		if vl.isStderr {
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

// stringWidth returns the visual width of a string, handling multi-byte/wide characters (C4).
func stringWidth(s string) int {
	return runewidth.StringWidth(s)
}

// truncateString truncates a string to maxWidth visual columns,
// accounting for multi-byte characters and wide runes.
func truncateString(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		rw := 1
		if r > 127 {
			rw = runewidth.RuneWidth(r)
		}
		if w+rw > maxWidth {
			return s[:i]
		}
		w += rw
	}
	return s
}

// truncateToWidth truncates a string to fit within width visual columns,
// correctly handling multi-byte UTF-8 and wide characters (C4).
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	stripped := process.StripAnsi(s)
	if stringWidth(stripped) <= width {
		return s + strings.Repeat(" ", width-stringWidth(stripped))
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
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		if visualPos+rw > width {
			break
		}
		result.WriteString(s[i : i+size])
		visualPos += rw
		i += size
	}
	// Pad remaining width
	if visualPos < width {
		result.WriteString(strings.Repeat(" ", width-visualPos))
	}
	// Reset any active ANSI styling
	result.WriteString("\x1b[0m")
	return result.String()
}
