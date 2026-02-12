package process

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

const MaxLogLines = 5000

// LogLine represents a single line in the log buffer.
type LogLine struct {
	Text      string
	Timestamp time.Time
	IsStderr  bool
}

// LogBuffer is a circular buffer of log lines for a single process.
type LogBuffer struct {
	mu        sync.Mutex
	Lines     []LogLine
	ScrollPos int
	Follow    bool
}

// NewLogBuffer creates an empty log buffer with follow mode enabled.
func NewLogBuffer() *LogBuffer {
	return &LogBuffer{
		Lines:  make([]LogLine, 0, 256),
		Follow: true,
	}
}

// Append adds text to the buffer, splitting on newlines.
// Returns the indices of newly added lines.
func (b *LogBuffer) Append(text string, isStderr bool) []int {
	b.mu.Lock()
	defer b.mu.Unlock()

	rawLines := splitLines(text)
	var indices []int

	for _, line := range rawLines {
		if len(line) == 0 {
			continue
		}
		clean := sanitizeLine(line)
		if len(clean) == 0 {
			continue
		}
		idx := len(b.Lines)
		b.Lines = append(b.Lines, LogLine{
			Text:      clean,
			Timestamp: time.Now(),
			IsStderr:  isStderr,
		})
		indices = append(indices, idx)
	}

	// Trim to max
	if len(b.Lines) > MaxLogLines {
		excess := len(b.Lines) - MaxLogLines
		// Copy to new slice to release old backing array memory
		newLines := make([]LogLine, MaxLogLines)
		copy(newLines, b.Lines[excess:])
		b.Lines = newLines
		b.ScrollPos -= excess
		if b.ScrollPos < 0 {
			b.ScrollPos = 0
		}
		// Clamp to valid max to prevent out-of-bounds after trim
		maxScroll := len(b.Lines)
		if b.ScrollPos > maxScroll {
			b.ScrollPos = maxScroll
		}
		// Adjust returned indices to account for trimmed lines (A1)
		for i := range indices {
			indices[i] -= excess
		}
		valid := indices[:0]
		for _, idx := range indices {
			if idx >= 0 {
				valid = append(valid, idx)
			}
		}
		indices = valid
	}

	return indices
}

// LineCount returns the number of lines in the buffer.
func (b *LogBuffer) LineCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.Lines)
}

// Clear removes all lines from the buffer.
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Lines = b.Lines[:0]
	b.ScrollPos = 0
	b.Follow = true
}

// ScrollTo sets the scroll position, clamping to valid range.
func (b *LogBuffer) ScrollTo(pos, viewHeight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scrollToLocked(pos, viewHeight)
}

func (b *LogBuffer) scrollToLocked(pos, viewHeight int) {
	maxScroll := len(b.Lines) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if pos < 0 {
		pos = 0
	}
	if pos > maxScroll {
		pos = maxScroll
	}
	b.ScrollPos = pos
	b.Follow = pos >= maxScroll
}

// ScrollBy adjusts scroll position by delta.
func (b *LogBuffer) ScrollBy(delta, viewHeight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.scrollToLocked(b.ScrollPos+delta, viewHeight)
}

// SnapToBottom moves scroll to the end and enables follow mode.
func (b *LogBuffer) SnapToBottom(viewHeight int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	maxScroll := len(b.Lines) - viewHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	b.ScrollPos = maxScroll
	b.Follow = true
}

// Snapshot returns a copy of the current lines and scroll state for safe rendering.
func (b *LogBuffer) Snapshot() (lines []LogLine, scrollPos int, follow bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]LogLine, len(b.Lines))
	copy(cp, b.Lines)
	return cp, b.ScrollPos, b.Follow
}

// GetLine returns the text of a line at the given index, or "" if out of range.
func (b *LogBuffer) GetLine(idx int) (LogLine, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if idx < 0 || idx >= len(b.Lines) {
		return LogLine{}, false
	}
	return b.Lines[idx], true
}

// GetLinesFrom returns lines starting at idx up to (but not including) the first blank line.
func (b *LogBuffer) GetLinesFrom(idx int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []string
	for i := idx; i < len(b.Lines); i++ {
		stripped := strings.TrimSpace(StripAnsi(b.Lines[i].Text))
		if len(stripped) == 0 {
			break
		}
		result = append(result, b.Lines[i].Text)
	}
	return result
}

func splitLines(s string) []string {
	// Split on \r\n, \n, or \r
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' {
			lines = append(lines, s[start:i])
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
			start = i + 1
		} else if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

var (
	// CSI cursor/screen control sequences
	csiControlRe = regexp.MustCompile(`\x1b\[\??[0-9;]*[HABCDEFGJKSTfhlr]`)
	// OSC sequences
	oscRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	// Non-CSI escapes
	nonCsiRe = regexp.MustCompile(`\x1b[^\[]\S?`)
	// ANSI color/style escape sequences
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
)

func sanitizeLine(s string) string {
	s = csiControlRe.ReplaceAllString(s, "")
	s = oscRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	s = nonCsiRe.ReplaceAllString(s, "")
	return s
}

// StripAnsi removes all ANSI escape codes from a string.
func StripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}
