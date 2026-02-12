package process

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ErrorPatterns matches common error output patterns.
var ErrorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bERROR\b`),
	regexp.MustCompile(`\bError:`),
	regexp.MustCompile(`\bException\b`),
	regexp.MustCompile(`(?i)\bFailed\b`),
	regexp.MustCompile(`(?i)\bFATAL\b`),
	regexp.MustCompile(`\bTypeError\b|\bReferenceError\b|\bSyntaxError\b`),
	regexp.MustCompile(`at\s+\S+\s+\([^)]+:\d+:\d+\)`),
	regexp.MustCompile(`^\s+at\s+`),
}

const maxStoredErrors = 50

// CapturedError holds the lines of a single detected error.
type CapturedError struct {
	Timestamp time.Time
	Lines     []string
}

// ErrorBuffer stores captured errors for a single process.
type ErrorBuffer struct {
	mu     sync.Mutex
	Errors []CapturedError
}

// redAnsiRe matches ANSI escape codes for red (31) and bright red (91) foreground.
var redAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*(31|91)m`)

// IsErrorLine returns true if a line matches error patterns or contains red ANSI coloring.
func IsErrorLine(line string) bool {
	return MatchesErrorPattern(line) || redAnsiRe.MatchString(line)
}

// MatchesErrorPattern checks if a line matches any error pattern.
func MatchesErrorPattern(line string) bool {
	stripped := StripAnsi(line)
	for _, re := range ErrorPatterns {
		if re.MatchString(stripped) {
			return true
		}
	}
	return false
}

// CaptureError captures an error starting at lineIdx in the log buffer,
// collecting lines until a blank line or end of buffer.
func (eb *ErrorBuffer) CaptureError(logBuf *LogBuffer, lineIdx int) {
	// Get lines from the log buffer using its lock
	errorLines := logBuf.GetLinesFrom(lineIdx)

	if len(errorLines) == 0 {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.Errors = append(eb.Errors, CapturedError{
		Timestamp: time.Now(),
		Lines:     errorLines,
	})

	if len(eb.Errors) > maxStoredErrors {
		eb.Errors = eb.Errors[1:]
	}
}

// Count returns the number of captured errors.
func (eb *ErrorBuffer) Count() int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return len(eb.Errors)
}

// LastErrorText returns the last captured error as plain text.
func (eb *ErrorBuffer) LastErrorText() string {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if len(eb.Errors) == 0 {
		return ""
	}
	last := eb.Errors[len(eb.Errors)-1]
	lines := make([]string, len(last.Lines))
	for i, l := range last.Lines {
		lines[i] = StripAnsi(l)
	}
	return strings.Join(lines, "\n")
}

// AllErrorsText returns all captured errors as plain text.
func (eb *ErrorBuffer) AllErrorsText() string {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if len(eb.Errors) == 0 {
		return ""
	}
	var parts []string
	for i, e := range eb.Errors {
		lines := make([]string, len(e.Lines))
		for j, l := range e.Lines {
			lines[j] = StripAnsi(l)
		}
		parts = append(parts, fmt.Sprintf("--- Error %d ---\n%s", i+1, strings.Join(lines, "\n")))
	}
	return strings.Join(parts, "\n\n")
}

// Clear removes all captured errors.
func (eb *ErrorBuffer) Clear() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.Errors = nil
}
