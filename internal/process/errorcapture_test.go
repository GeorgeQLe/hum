package process

import (
	"testing"
	"time"
)

func makeLines(texts []string) []LogLine {
	now := time.Now()
	lines := make([]LogLine, len(texts))
	for i, text := range texts {
		lines[i] = LogLine{
			Text:      text,
			Timestamp: now.Add(time.Duration(i) * 100 * time.Millisecond),
		}
	}
	return lines
}

func TestCaptureBackward_BlankLine(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"normal log line",     // 0
		"another log line",    // 1
		"",                    // 2 (blank)
		"error context line",  // 3
		"TypeError: bad call", // 4 (trigger)
	})

	start := bd.CaptureBackward(lines, 4)
	if start != 3 {
		t.Errorf("expected start=3, got %d", start)
	}
}

func TestCaptureBackward_Boundary(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"2024-01-15T10:30:00 INFO Starting...", // 0 (boundary)
		"loading module...",                     // 1
		"TypeError: bad call",                   // 2 (trigger)
	})

	start := bd.CaptureBackward(lines, 2)
	// Should stop at line 0 (boundary) and not include it
	if start != 1 {
		t.Errorf("expected start=1, got %d", start)
	}
}

func TestCaptureBackward_MaxLines(t *testing.T) {
	bd := NewBoundaryDetector()
	texts := make([]string, 30)
	for i := range texts {
		texts[i] = "some context line"
	}
	texts[29] = "TypeError: trigger"
	lines := makeLines(texts)

	start := bd.CaptureBackward(lines, 29)
	// Should be limited to maxBackwardLines (20)
	if start < 29-maxBackwardLines {
		t.Errorf("expected start >= %d, got %d", 29-maxBackwardLines, start)
	}
}

func TestCaptureForward_BlankLine(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"TypeError: bad call",       // 0 (trigger)
		"    at foo (file.js:10:5)", // 1
		"    at bar (file.js:20:3)", // 2
		"",                          // 3 (blank)
		"normal log line",           // 4
	})

	end := bd.CaptureForward(lines, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestCaptureForward_Continuation(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"Error: something broke",    // 0 (trigger)
		"    at foo (file.js:10:5)", // 1
		"",                          // 2 (blank)
		"    at bar (file.js:20:3)", // 3 (continuation past blank)
		"normal output",             // 4
	})

	end := bd.CaptureForward(lines, 0)
	if end != 4 {
		t.Errorf("expected end=4 (continuation past blank), got %d", end)
	}
}

func TestCaptureForward_CausedBy(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"Exception: outer error",     // 0 (trigger)
		"    at outer (file.js:1:1)", // 1
		"",                           // 2 (blank)
		"Caused by: inner error",     // 3 (continuation)
		"    at inner (file.js:5:1)", // 4
		"",                           // 5
		"normal log",                 // 6
	})

	end := bd.CaptureForward(lines, 0)
	if end < 5 {
		t.Errorf("expected end >= 5 (should include Caused by block), got %d", end)
	}
}

func TestProcessBatch_SingleTrigger(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"normal line",               // 0
		"",                          // 1
		"TypeError: bad",            // 2
		"    at fn (file.js:10:5)",  // 3
		"",                          // 4
	})

	ranges := bd.ProcessBatch(lines, []int{2})
	if len(ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(ranges))
	}
	if ranges[0].start != 2 || ranges[0].end != 4 {
		t.Errorf("expected range [2,4), got [%d,%d)", ranges[0].start, ranges[0].end)
	}
}

func TestProcessBatch_OverlappingTriggers(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"Error: first line",         // 0
		"    at fn (file.js:1:1)",   // 1 (also a trigger via stack pattern)
		"    at fn2 (file.js:2:1)",  // 2
		"",                          // 3
	})

	// Both 0 and 1 are triggers, their captures should merge
	ranges := bd.ProcessBatch(lines, []int{0, 1})
	if len(ranges) != 1 {
		t.Fatalf("expected 1 merged range, got %d", len(ranges))
	}
}

func TestProcessBatch_DisjointTriggers(t *testing.T) {
	bd := NewBoundaryDetector()
	lines := makeLines([]string{
		"Error: first",              // 0
		"",                          // 1
		"normal output",             // 2
		"",                          // 3
		"Error: second",             // 4
		"",                          // 5
	})

	ranges := bd.ProcessBatch(lines, []int{0, 4})
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}
}

func TestMergeRanges(t *testing.T) {
	tests := []struct {
		name     string
		input    []captureRange
		expected []captureRange
	}{
		{
			name:     "empty",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single",
			input:    []captureRange{{0, 5}},
			expected: []captureRange{{0, 5}},
		},
		{
			name:     "non-overlapping",
			input:    []captureRange{{0, 3}, {5, 8}},
			expected: []captureRange{{0, 3}, {5, 8}},
		},
		{
			name:     "overlapping",
			input:    []captureRange{{0, 5}, {3, 8}},
			expected: []captureRange{{0, 8}},
		},
		{
			name:     "adjacent",
			input:    []captureRange{{0, 5}, {5, 8}},
			expected: []captureRange{{0, 8}},
		},
		{
			name:     "contained",
			input:    []captureRange{{0, 10}, {3, 7}},
			expected: []captureRange{{0, 10}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeRanges(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d ranges, got %d: %v", len(tt.expected), len(result), result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("range[%d]: expected %v, got %v", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestExtractLines(t *testing.T) {
	lines := makeLines([]string{"a", "b", "c", "d", "e"})
	result := ExtractLines(lines, 1, 4)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
	if result[0] != "b" || result[1] != "c" || result[2] != "d" {
		t.Errorf("unexpected lines: %v", result)
	}
}
