package process

import (
	"regexp"
	"strings"
	"time"
)

const (
	maxBackwardLines = 20
	maxForwardLines  = 50
)

// Patterns that indicate continuation of an error across blank lines.
var continuationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s+at\s+`),                 // stack frames
	regexp.MustCompile(`(?i)^\s*Caused by:`),         // Java-style chained errors
	regexp.MustCompile(`(?i)^\s+\d+\s*\|`),          // source code context lines
	regexp.MustCompile(`^\s+~+$`),                   // TypeScript squiggly underlines
	regexp.MustCompile(`^\s+\^`),                    // caret markers
	regexp.MustCompile(`(?i)^\s+error\s+TS\d+`),     // additional TS diagnostics
}

// Patterns that indicate a boundary (start of a new log entry).
var boundaryPatterns = []*regexp.Regexp{
	// Timestamp prefixes like "2024-01-15T10:30:00" or "[10:30:00]"
	regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]`),
	regexp.MustCompile(`^\[\d{2}:\d{2}:\d{2}`),
	// Log level changes
	regexp.MustCompile(`(?i)^\s*\[(INFO|WARN|DEBUG|TRACE)\]`),
	regexp.MustCompile(`(?i)^(INFO|WARN|DEBUG|TRACE)\s*[:\|]`),
}

// BoundaryDetector determines error capture boundaries using
// configurable boundary and continuation markers.
type BoundaryDetector struct {
	boundaries    []*regexp.Regexp
	continuations []*regexp.Regexp
}

// NewBoundaryDetector creates a BoundaryDetector with default patterns.
func NewBoundaryDetector() *BoundaryDetector {
	return &BoundaryDetector{
		boundaries:    boundaryPatterns,
		continuations: continuationPatterns,
	}
}

// isBoundary returns true if the line marks the start of a new log entry.
func (bd *BoundaryDetector) isBoundary(line string) bool {
	stripped := StripAnsi(line)
	for _, re := range bd.boundaries {
		if re.MatchString(stripped) {
			return true
		}
	}
	return false
}

// isContinuation returns true if the line is a continuation of the current error
// (e.g., stack frame, "Caused by:", source context).
func (bd *BoundaryDetector) isContinuation(line string) bool {
	stripped := StripAnsi(line)
	for _, re := range bd.continuations {
		if re.MatchString(stripped) {
			return true
		}
	}
	return false
}

// CaptureBackward walks backward from triggerIdx to find the error start.
// Stops at the first boundary marker, blank line, or maxBackwardLines.
// Returns the start index (inclusive).
func (bd *BoundaryDetector) CaptureBackward(lines []LogLine, triggerIdx int) int {
	start := triggerIdx
	limit := triggerIdx - maxBackwardLines
	if limit < 0 {
		limit = 0
	}

	for i := triggerIdx - 1; i >= limit; i-- {
		stripped := strings.TrimSpace(StripAnsi(lines[i].Text))

		// Stop at blank line
		if stripped == "" {
			break
		}

		// Stop at boundary (new log entry)
		if bd.isBoundary(lines[i].Text) {
			// Only stop if this isn't a continuation of the same error
			if !bd.isContinuation(lines[i].Text) {
				break
			}
		}

		// Stop if timestamp jump > 2 seconds (likely different log entry)
		if !lines[i].Timestamp.IsZero() && !lines[triggerIdx].Timestamp.IsZero() {
			if lines[triggerIdx].Timestamp.Sub(lines[i].Timestamp) > 2*time.Second {
				break
			}
		}

		start = i
	}

	return start
}

// CaptureForward walks forward from triggerIdx to find the error end.
// Continues past blank lines if the next non-blank line is a continuation pattern.
// After seeing continuation lines (stack frames, "Caused by:"), stops when
// a non-continuation line is encountered.
// Returns the end index (exclusive).
func (bd *BoundaryDetector) CaptureForward(lines []LogLine, triggerIdx int) int {
	end := triggerIdx + 1
	limit := triggerIdx + maxForwardLines
	if limit > len(lines) {
		limit = len(lines)
	}

	blankCount := 0
	inContinuation := false
	for i := triggerIdx + 1; i < limit; i++ {
		stripped := strings.TrimSpace(StripAnsi(lines[i].Text))

		if stripped == "" {
			blankCount++
			// Allow up to 1 blank line if followed by continuation
			if blankCount > 1 {
				break
			}
			// Peek ahead for continuation
			if i+1 < limit && bd.isContinuation(lines[i+1].Text) {
				end = i + 1
				continue
			}
			break
		}

		blankCount = 0

		// Stop at boundary that isn't a continuation
		if bd.isBoundary(lines[i].Text) && !bd.isContinuation(lines[i].Text) {
			break
		}

		// Stop if timestamp jump > 2 seconds
		if !lines[i].Timestamp.IsZero() && !lines[triggerIdx].Timestamp.IsZero() {
			if lines[i].Timestamp.Sub(lines[triggerIdx].Timestamp) > 2*time.Second {
				break
			}
		}

		// Stack frames and continuations always extend
		if bd.isContinuation(lines[i].Text) {
			inContinuation = true
			end = i + 1
			continue
		}

		// If we were in a continuation section and hit a non-continuation line, stop
		if inContinuation {
			break
		}

		end = i + 1
	}

	return end
}

// captureRange holds the start/end indices for a captured error block.
type captureRange struct {
	start int // inclusive
	end   int // exclusive
}

// ProcessBatch processes a batch of trigger indices, deduplicating overlapping captures.
// Returns a list of non-overlapping captured error blocks.
func (bd *BoundaryDetector) ProcessBatch(lines []LogLine, triggerIndices []int) []captureRange {
	if len(triggerIndices) == 0 {
		return nil
	}

	var ranges []captureRange

	for _, idx := range triggerIndices {
		if idx < 0 || idx >= len(lines) {
			continue
		}
		start := bd.CaptureBackward(lines, idx)
		end := bd.CaptureForward(lines, idx)
		ranges = append(ranges, captureRange{start: start, end: end})
	}

	// Merge overlapping ranges
	return mergeRanges(ranges)
}

// mergeRanges merges overlapping or adjacent capture ranges.
func mergeRanges(ranges []captureRange) []captureRange {
	if len(ranges) <= 1 {
		return ranges
	}

	// Simple insertion sort since ranges are typically small and mostly sorted
	for i := 1; i < len(ranges); i++ {
		key := ranges[i]
		j := i - 1
		for j >= 0 && ranges[j].start > key.start {
			ranges[j+1] = ranges[j]
			j--
		}
		ranges[j+1] = key
	}

	merged := []captureRange{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		if ranges[i].start <= last.end {
			// Overlapping or adjacent — extend
			if ranges[i].end > last.end {
				last.end = ranges[i].end
			}
		} else {
			merged = append(merged, ranges[i])
		}
	}

	return merged
}

// ExtractLines extracts the text from a range of LogLines.
func ExtractLines(lines []LogLine, start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	result := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		result = append(result, lines[i].Text)
	}
	return result
}
