package process

import "regexp"

// ExclusionPatterns matches lines that should NOT be flagged as errors
// even if they match an error trigger pattern (false positive suppression).
var ExclusionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b0\s+errors?\b`),
	regexp.MustCompile(`(?i)error\s*handl`),
	regexp.MustCompile(`(?i)Failed to find cached`),
	regexp.MustCompile(`(?i)error.?free`),
	regexp.MustCompile(`(?i)\bno\s+errors?\b`),
	regexp.MustCompile(`(?i)(?:if|on|handle)\s*.*error`),
	regexp.MustCompile(`(?i)warning:\s*.*deprecated`),
}

// ErrorDetector combines trigger patterns with exclusion patterns
// to reduce false positives in error detection.
type ErrorDetector struct {
	triggers   []*regexp.Regexp
	exclusions []*regexp.Regexp
	redAnsi    *regexp.Regexp
}

// NewErrorDetector creates an ErrorDetector with the default trigger
// and exclusion pattern sets.
func NewErrorDetector() *ErrorDetector {
	return &ErrorDetector{
		triggers:   ErrorPatterns,
		exclusions: ExclusionPatterns,
		redAnsi:    redAnsiRe,
	}
}

// IsError returns true if the line matches a trigger pattern or contains
// red ANSI coloring, AND does not match any exclusion pattern.
func (d *ErrorDetector) IsError(line string) bool {
	stripped := StripAnsi(line)

	// Check exclusions first — if any match, suppress
	for _, ex := range d.exclusions {
		if ex.MatchString(stripped) {
			return false
		}
	}

	// Check triggers
	for _, re := range d.triggers {
		if re.MatchString(stripped) {
			return true
		}
	}

	// Check red ANSI coloring on original line
	return d.redAnsi.MatchString(line)
}
