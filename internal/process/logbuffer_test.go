package process

import (
	"strings"
	"testing"
)

func TestLogBufferAppend(t *testing.T) {
	buf := NewLogBuffer()

	indices := buf.Append("hello world\n", false)
	if len(indices) != 1 {
		t.Fatalf("Append returned %d indices, want 1", len(indices))
	}
	if buf.LineCount() != 1 {
		t.Fatalf("LineCount = %d, want 1", buf.LineCount())
	}
	if buf.Lines[0].Text != "hello world" {
		t.Errorf("Line text = %q, want %q", buf.Lines[0].Text, "hello world")
	}
}

func TestLogBufferMultiline(t *testing.T) {
	buf := NewLogBuffer()

	buf.Append("line1\nline2\nline3\n", false)
	if buf.LineCount() != 3 {
		t.Fatalf("LineCount = %d, want 3", buf.LineCount())
	}
	if buf.Lines[0].Text != "line1" {
		t.Errorf("Line[0] = %q, want %q", buf.Lines[0].Text, "line1")
	}
	if buf.Lines[2].Text != "line3" {
		t.Errorf("Line[2] = %q, want %q", buf.Lines[2].Text, "line3")
	}
}

func TestLogBufferMaxLines(t *testing.T) {
	buf := NewLogBuffer()

	// Add more than MaxLogLines
	for i := 0; i < MaxLogLines+100; i++ {
		buf.Append("line\n", false)
	}

	if buf.LineCount() != MaxLogLines {
		t.Errorf("LineCount = %d, want %d", buf.LineCount(), MaxLogLines)
	}
}

func TestLogBufferStderr(t *testing.T) {
	buf := NewLogBuffer()

	buf.Append("error message\n", true)
	if !buf.Lines[0].IsStderr {
		t.Error("Expected IsStderr to be true")
	}
}

func TestLogBufferScroll(t *testing.T) {
	buf := NewLogBuffer()
	for i := 0; i < 100; i++ {
		buf.Append("line\n", false)
	}

	buf.ScrollTo(50, 20)
	if buf.ScrollPos != 50 {
		t.Errorf("ScrollPos = %d, want 50", buf.ScrollPos)
	}
	if buf.Follow {
		t.Error("Expected Follow to be false after scrolling up")
	}

	buf.SnapToBottom(20)
	if !buf.Follow {
		t.Error("Expected Follow to be true after SnapToBottom")
	}
	if buf.ScrollPos != 80 {
		t.Errorf("ScrollPos = %d, want 80", buf.ScrollPos)
	}
}

func TestLogBufferScrollBy(t *testing.T) {
	buf := NewLogBuffer()
	for i := 0; i < 100; i++ {
		buf.Append("line\n", false)
	}

	// Start at the bottom (Follow mode puts us at maxScroll=80)
	buf.SnapToBottom(20)
	if buf.ScrollPos != 80 {
		t.Fatalf("initial ScrollPos = %d, want 80", buf.ScrollPos)
	}

	buf.ScrollBy(-10, 20)
	if buf.ScrollPos != 70 {
		t.Errorf("ScrollPos = %d, want 70", buf.ScrollPos)
	}
}

func TestLogBufferClear(t *testing.T) {
	buf := NewLogBuffer()
	buf.Append("test\n", false)
	buf.Clear()

	if buf.LineCount() != 0 {
		t.Errorf("LineCount = %d after Clear, want 0", buf.LineCount())
	}
	if !buf.Follow {
		t.Error("Expected Follow to be true after Clear")
	}
}

func TestSanitizeLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"cursor move", "\x1b[10;5Hhello", "hello"},
		{"erase line", "\x1b[2Khello", "hello"},
		{"carriage return", "hello\rworld", "helloworld"},
		{"preserves color codes", "\x1b[31mhello\x1b[0m", "\x1b[31mhello\x1b[0m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeLine(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"\x1b[31mhello\x1b[0m", "hello"},
		{"\x1b[1m\x1b[32mgreen bold\x1b[0m", "green bold"},
	}

	for _, tt := range tests {
		got := StripAnsi(tt.input)
		if got != tt.want {
			t.Errorf("StripAnsi(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"one\ntwo\nthree", 3},
		{"one\r\ntwo\r\nthree", 3},
		{"single", 1},
		{"", 0},
		{"trailing\n", 1}, // "trailing" only, empty string after \n is not emitted
	}

	for _, tt := range tests {
		got := splitLines(tt.input)
		// Filter out empty last element for trailing newline comparison
		nonEmpty := 0
		for _, s := range got {
			if s != "" {
				nonEmpty++
			}
		}
		_ = nonEmpty
		if len(got) != tt.want {
			t.Errorf("splitLines(%q) returned %d parts (%v), want %d", tt.input, len(got), got, tt.want)
		}
	}
}

func TestLogBufferEmptyLines(t *testing.T) {
	buf := NewLogBuffer()
	buf.Append("\n\n\n", false)
	// Empty lines should be skipped
	if buf.LineCount() != 0 {
		t.Errorf("LineCount = %d, want 0 (empty lines should be skipped)", buf.LineCount())
	}
}

func TestLogBufferLargeInput(t *testing.T) {
	buf := NewLogBuffer()
	// Simulate a large burst of log output
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("log line content here\n")
	}
	buf.Append(sb.String(), false)

	if buf.LineCount() != 1000 {
		t.Errorf("LineCount = %d, want 1000", buf.LineCount())
	}
}
