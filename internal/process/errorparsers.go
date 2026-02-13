package process

import (
	"regexp"
	"strconv"
	"strings"
)

// ErrorParser can detect and parse structured error information from raw lines.
type ErrorParser interface {
	CanParse(lines []string) bool
	Parse(lines []string) *ParsedError
}

// --- V8 Error Parser ---

var (
	// Matches "TypeError: msg", "ReferenceError: msg", etc.
	v8ErrorLineRe = regexp.MustCompile(`^(\w*(?:Error|Exception)):\s*(.+)`)
	// Matches "    at fn (file:line:col)" or "    at file:line:col"
	v8StackFrameRe = regexp.MustCompile(`^\s+at\s+(?:(.+?)\s+\((.+?):(\d+):(\d+)\)|(.+?):(\d+):(\d+))`)
)

type v8ErrorParser struct{}

func (p *v8ErrorParser) CanParse(lines []string) bool {
	for _, line := range lines {
		stripped := StripAnsi(line)
		if v8ErrorLineRe.MatchString(stripped) {
			return true
		}
	}
	return false
}

func (p *v8ErrorParser) Parse(lines []string) *ParsedError {
	pe := &ParsedError{
		Kind:       ErrorV8,
		RawLines:   lines,
		PlainLines: stripAll(lines),
	}

	for i, line := range pe.PlainLines {
		if m := v8ErrorLineRe.FindStringSubmatch(line); m != nil {
			pe.ErrorType = m[1]
			pe.Message = m[2]

			// Collect stack trace from subsequent lines
			for j := i + 1; j < len(pe.PlainLines); j++ {
				if v8StackFrameRe.MatchString(pe.PlainLines[j]) {
					pe.StackTrace = append(pe.StackTrace, pe.PlainLines[j])
				}
			}

			// Extract location from first stack frame
			if len(pe.StackTrace) > 0 {
				pe.Location = parseV8Frame(pe.StackTrace[0])
			}
			return pe
		}
	}

	// Fallback
	if len(pe.PlainLines) > 0 {
		pe.Message = pe.PlainLines[0]
	}
	return pe
}

func parseV8Frame(frame string) *SourceLocation {
	m := v8StackFrameRe.FindStringSubmatch(frame)
	if m == nil {
		return nil
	}

	// "at fn (file:line:col)" form
	if m[2] != "" {
		line, _ := strconv.Atoi(m[3])
		col, _ := strconv.Atoi(m[4])
		return &SourceLocation{File: m[2], Line: line, Column: col}
	}

	// "at file:line:col" form
	if m[5] != "" {
		line, _ := strconv.Atoi(m[6])
		col, _ := strconv.Atoi(m[7])
		return &SourceLocation{File: m[5], Line: line, Column: col}
	}

	return nil
}

// --- TypeScript Compiler Parser ---

var (
	// Matches "file.ts(line,col): error TS2322: msg"
	tsErrorRe = regexp.MustCompile(`^(.+?)\((\d+),(\d+)\):\s*error\s+(TS\d+):\s*(.+)`)
)

type tsCompilerParser struct{}

func (p *tsCompilerParser) CanParse(lines []string) bool {
	for _, line := range lines {
		stripped := StripAnsi(line)
		if tsErrorRe.MatchString(stripped) {
			return true
		}
	}
	return false
}

func (p *tsCompilerParser) Parse(lines []string) *ParsedError {
	pe := &ParsedError{
		Kind:       ErrorTSCompiler,
		RawLines:   lines,
		PlainLines: stripAll(lines),
	}

	for _, line := range pe.PlainLines {
		if m := tsErrorRe.FindStringSubmatch(line); m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			pe.ErrorType = m[4]
			pe.Message = m[5]
			pe.Location = &SourceLocation{
				File:   m[1],
				Line:   lineNum,
				Column: col,
			}
			return pe
		}
	}

	if len(pe.PlainLines) > 0 {
		pe.Message = pe.PlainLines[0]
	}
	return pe
}

// --- Bundler Error Parser ---

var (
	// Vite: "[vite] error message" or "[vite] Internal server error: ..."
	viteErrorRe = regexp.MustCompile(`\[vite\]\s*(?:Internal server error:\s*)?(.+)`)
	// esbuild: "X [ERROR] msg"
	esbuildErrorRe = regexp.MustCompile(`^✘\s*\[ERROR\]\s*(.+)`)
	// webpack: "ERROR in file"
	webpackErrorRe = regexp.MustCompile(`^ERROR\s+in\s+(.+)`)
	// Generic file:line:col pattern for location extraction
	fileLocRe = regexp.MustCompile(`(\S+?):(\d+):(\d+)`)
)

type bundlerErrorParser struct{}

func (p *bundlerErrorParser) CanParse(lines []string) bool {
	for _, line := range lines {
		stripped := StripAnsi(line)
		if viteErrorRe.MatchString(stripped) || esbuildErrorRe.MatchString(stripped) || webpackErrorRe.MatchString(stripped) {
			return true
		}
	}
	return false
}

func (p *bundlerErrorParser) Parse(lines []string) *ParsedError {
	pe := &ParsedError{
		Kind:       ErrorBundler,
		ErrorType:  "Build Error",
		RawLines:   lines,
		PlainLines: stripAll(lines),
	}

	for _, line := range pe.PlainLines {
		if m := viteErrorRe.FindStringSubmatch(line); m != nil {
			pe.Message = m[1]
			pe.ErrorType = "Vite Error"
			pe.Location = extractFileLocation(pe.PlainLines)
			return pe
		}
		if m := esbuildErrorRe.FindStringSubmatch(line); m != nil {
			pe.Message = m[1]
			pe.ErrorType = "esbuild Error"
			pe.Location = extractFileLocation(pe.PlainLines)
			return pe
		}
		if m := webpackErrorRe.FindStringSubmatch(line); m != nil {
			pe.Message = m[1]
			pe.ErrorType = "webpack Error"
			pe.Location = extractFileLocation(pe.PlainLines)
			return pe
		}
	}

	if len(pe.PlainLines) > 0 {
		pe.Message = pe.PlainLines[0]
	}
	return pe
}

// --- Generic Error Parser ---

type genericErrorParser struct{}

func (p *genericErrorParser) CanParse(_ []string) bool {
	return true // always matches as fallback
}

func (p *genericErrorParser) Parse(lines []string) *ParsedError {
	pe := &ParsedError{
		Kind:       ErrorGeneric,
		RawLines:   lines,
		PlainLines: stripAll(lines),
	}

	if len(pe.PlainLines) > 0 {
		pe.Message = pe.PlainLines[0]
		// Try to extract error type from the message
		if m := v8ErrorLineRe.FindStringSubmatch(pe.PlainLines[0]); m != nil {
			pe.ErrorType = m[1]
			pe.Message = m[2]
		}
	}

	// Scan all lines for file:line:col
	pe.Location = extractFileLocation(pe.PlainLines)

	// Collect any stack-like lines
	for _, line := range pe.PlainLines {
		if strings.HasPrefix(strings.TrimSpace(line), "at ") {
			pe.StackTrace = append(pe.StackTrace, line)
		}
	}

	return pe
}

// --- Parser registry ---

// defaultParsers is the priority-ordered list of error parsers.
var defaultParsers = []ErrorParser{
	&v8ErrorParser{},
	&tsCompilerParser{},
	&bundlerErrorParser{},
	&genericErrorParser{},
}

// ParseError tries each parser in priority order and returns the first successful parse.
func ParseError(lines []string) *ParsedError {
	for _, parser := range defaultParsers {
		if parser.CanParse(lines) {
			return parser.Parse(lines)
		}
	}
	return nil
}

// --- Helpers ---

func stripAll(lines []string) []string {
	result := make([]string, len(lines))
	for i, l := range lines {
		result[i] = StripAnsi(l)
	}
	return result
}

func extractFileLocation(lines []string) *SourceLocation {
	for _, line := range lines {
		if m := fileLocRe.FindStringSubmatch(line); m != nil {
			lineNum, _ := strconv.Atoi(m[2])
			col, _ := strconv.Atoi(m[3])
			// Skip matches that look like timestamps or URLs
			file := m[1]
			if strings.Contains(file, "://") || strings.HasPrefix(file, "http") {
				continue
			}
			return &SourceLocation{File: file, Line: lineNum, Column: col}
		}
	}
	return nil
}
