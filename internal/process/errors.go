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

const maxStoredErrors = 100

// SourceLocation holds a parsed file:line:col reference.
type SourceLocation struct {
	File   string
	Line   int
	Column int
}

// String formats the location as file:line:col (or file:line if col is 0).
func (s *SourceLocation) String() string {
	if s == nil {
		return ""
	}
	if s.Column > 0 {
		return fmt.Sprintf("%s:%d:%d", s.File, s.Line, s.Column)
	}
	return fmt.Sprintf("%s:%d", s.File, s.Line)
}

// ErrorKind classifies the origin of a parsed error.
type ErrorKind int

const (
	ErrorGeneric    ErrorKind = iota
	ErrorV8                   // V8/Node.js runtime errors
	ErrorTSCompiler           // TypeScript compiler diagnostics
	ErrorBundler              // Vite, esbuild, webpack errors
)

// ParsedError holds structured information extracted from raw error lines.
type ParsedError struct {
	Kind       ErrorKind
	ErrorType  string          // "TypeError", "TS2322", "Build Error", etc.
	Message    string          // core error message
	Location   *SourceLocation // primary source location, if any
	StackTrace []string        // stack frame lines
	RawLines   []string        // ANSI preserved
	PlainLines []string        // ANSI stripped
}

// DedupKey returns a deduplication key based on error type and message.
func (p *ParsedError) DedupKey() string {
	return p.ErrorType + ":" + p.Message
}

// ErrorGroup aggregates duplicate errors by their dedup key.
type ErrorGroup struct {
	Key       string         // ErrorType+Message
	Count     int            // number of occurrences
	Latest    *CapturedError // most recent occurrence
	FirstSeen time.Time
	LastSeen  time.Time
}

// CapturedError holds the lines of a single detected error.
type CapturedError struct {
	Timestamp time.Time
	Lines     []string
	AppName   string       // which app produced this error
	Parsed    *ParsedError // structured parse result (may be nil)
	dedupKey  string       // cached dedup key
}

// ErrorBuffer stores captured errors for a single process.
type ErrorBuffer struct {
	mu     sync.Mutex
	Errors []CapturedError
	groups map[string][]int // dedupKey -> indices into Errors
}

// redAnsiRe matches ANSI escape codes for red (31) and bright red (91) foreground.
var redAnsiRe = regexp.MustCompile(`\x1b\[(0;)?(1;)?(31|91)m`)

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
		newErrors := make([]CapturedError, len(eb.Errors)-1)
		copy(newErrors, eb.Errors[1:])
		eb.Errors = newErrors
		// Rebuild group index after trimming
		eb.rebuildGroupsLocked()
	}
}

// AddParsedError captures an error with structured parsing and deduplication.
func (eb *ErrorBuffer) AddParsedError(appName string, rawLines []string, parsed *ParsedError) {
	if len(rawLines) == 0 {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	key := ""
	if parsed != nil {
		key = parsed.DedupKey()
	}

	ce := CapturedError{
		Timestamp: time.Now(),
		Lines:     rawLines,
		AppName:   appName,
		Parsed:    parsed,
		dedupKey:  key,
	}

	eb.Errors = append(eb.Errors, ce)
	idx := len(eb.Errors) - 1

	// Update group index
	if key != "" {
		if eb.groups == nil {
			eb.groups = make(map[string][]int)
		}
		eb.groups[key] = append(eb.groups[key], idx)
	}

	if len(eb.Errors) > maxStoredErrors {
		newErrors := make([]CapturedError, len(eb.Errors)-1)
		copy(newErrors, eb.Errors[1:])
		eb.Errors = newErrors
		eb.rebuildGroupsLocked()
	}
}

// rebuildGroupsLocked rebuilds the group index from scratch.
// Must be called with eb.mu held.
func (eb *ErrorBuffer) rebuildGroupsLocked() {
	eb.groups = make(map[string][]int)
	for i, e := range eb.Errors {
		if e.dedupKey != "" {
			eb.groups[e.dedupKey] = append(eb.groups[e.dedupKey], i)
		}
	}
}

// GroupedErrors returns errors grouped by dedup key, ordered by last seen.
func (eb *ErrorBuffer) GroupedErrors() []ErrorGroup {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return eb.groupedErrorsLocked()
}

func (eb *ErrorBuffer) groupedErrorsLocked() []ErrorGroup {
	if len(eb.Errors) == 0 {
		return nil
	}

	seen := make(map[string]*ErrorGroup)
	var order []string

	for i := range eb.Errors {
		e := &eb.Errors[i]
		key := e.dedupKey
		if key == "" {
			// Ungrouped errors get a unique key
			key = fmt.Sprintf("_ungrouped_%d", i)
		}

		if g, ok := seen[key]; ok {
			g.Count++
			g.Latest = e
			g.LastSeen = e.Timestamp
		} else {
			seen[key] = &ErrorGroup{
				Key:       key,
				Count:     1,
				Latest:    e,
				FirstSeen: e.Timestamp,
				LastSeen:  e.Timestamp,
			}
			order = append(order, key)
		}
	}

	result := make([]ErrorGroup, 0, len(order))
	for _, key := range order {
		result = append(result, *seen[key])
	}
	return result
}

// SnapshotErrors returns a copy of all captured errors, safe for concurrent use.
func (eb *ErrorBuffer) SnapshotErrors() []CapturedError {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	result := make([]CapturedError, len(eb.Errors))
	copy(result, eb.Errors)
	return result
}

// Count returns the number of captured errors.
func (eb *ErrorBuffer) Count() int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	return len(eb.Errors)
}

// UniqueCount returns the number of unique error groups.
func (eb *ErrorBuffer) UniqueCount() int {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	groups := eb.groupedErrorsLocked()
	return len(groups)
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
	eb.groups = nil
}
