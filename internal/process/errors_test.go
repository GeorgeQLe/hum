package process

import (
	"strings"
	"testing"
)

func TestMatchesErrorPattern(t *testing.T) {
	tests := []struct {
		line  string
		match bool
	}{
		{"ERROR: something went wrong", true},
		{"Error: file not found", true},
		{"java.lang.NullPointer Exception thrown", true},
		{"Build Failed", true},
		{"FATAL error in module", true},
		{"TypeError: undefined is not a function", true},
		{"at Module._compile (/path/file.js:10:20)", true},
		{"    at Object.<anonymous>", true},
		{"this is a normal log line", false},
		{"Starting server on port 3000", false},
		{"Compilation successful", false},
		{"", false},
	}

	for _, tt := range tests {
		got := MatchesErrorPattern(tt.line)
		if got != tt.match {
			t.Errorf("MatchesErrorPattern(%q) = %v, want %v", tt.line, got, tt.match)
		}
	}
}

func TestCaptureError(t *testing.T) {
	logBuf := NewLogBuffer()
	logBuf.Append("Starting app...", false)
	logBuf.Append("Error: something broke", false)
	logBuf.Append("  at module.js:10", false)
	logBuf.Append("", false) // blank line ends capture

	eb := &ErrorBuffer{}
	eb.CaptureError(logBuf, 1) // Start from "Error: something broke"

	if eb.Count() != 1 {
		t.Fatalf("expected 1 error, got %d", eb.Count())
	}

	text := eb.LastErrorText()
	if !strings.Contains(text, "Error: something broke") {
		t.Errorf("expected error text to contain 'Error: something broke', got: %s", text)
	}
	if !strings.Contains(text, "at module.js:10") {
		t.Errorf("expected error text to contain stack trace, got: %s", text)
	}
}

func TestErrorBufferMax(t *testing.T) {
	eb := &ErrorBuffer{}
	logBuf := NewLogBuffer()

	for i := 0; i < 120; i++ {
		logBuf.Append("Error: test error", false)
		eb.CaptureError(logBuf, logBuf.LineCount()-1)
	}

	if eb.Count() > maxStoredErrors {
		t.Errorf("expected max %d errors, got %d", maxStoredErrors, eb.Count())
	}
}

func TestLastErrorText(t *testing.T) {
	eb := &ErrorBuffer{}
	logBuf := NewLogBuffer()

	logBuf.Append("Error: first", false)
	eb.CaptureError(logBuf, 0)

	logBuf.Append("Error: second", false)
	eb.CaptureError(logBuf, logBuf.LineCount()-1)

	text := eb.LastErrorText()
	if !strings.Contains(text, "Error: second") {
		t.Errorf("expected last error text to contain 'Error: second', got: %s", text)
	}
}

func TestAllErrorsText(t *testing.T) {
	eb := &ErrorBuffer{}
	logBuf := NewLogBuffer()

	logBuf.Append("Error: first", false)
	logBuf.Append("", false) // separator
	eb.CaptureError(logBuf, 0)

	logBuf.Append("Error: second", false)
	eb.CaptureError(logBuf, logBuf.LineCount()-1)

	text := eb.AllErrorsText()
	if !strings.Contains(text, "--- Error 1 ---") {
		t.Errorf("expected '--- Error 1 ---' in output, got: %s", text)
	}
	if !strings.Contains(text, "--- Error 2 ---") {
		t.Errorf("expected '--- Error 2 ---' in output, got: %s", text)
	}
	if !strings.Contains(text, "Error: first") {
		t.Errorf("expected 'Error: first' in output, got: %s", text)
	}
	if !strings.Contains(text, "Error: second") {
		t.Errorf("expected 'Error: second' in output, got: %s", text)
	}
}

func TestErrorBufferClear(t *testing.T) {
	eb := &ErrorBuffer{}
	logBuf := NewLogBuffer()

	logBuf.Append("Error: test", false)
	eb.CaptureError(logBuf, 0)

	if eb.Count() != 1 {
		t.Fatalf("expected 1 error, got %d", eb.Count())
	}

	eb.Clear()
	if eb.Count() != 0 {
		t.Errorf("expected 0 errors after clear, got %d", eb.Count())
	}
}

func TestEmptyErrorBuffer(t *testing.T) {
	eb := &ErrorBuffer{}
	if text := eb.LastErrorText(); text != "" {
		t.Errorf("expected empty string, got: %s", text)
	}
	if text := eb.AllErrorsText(); text != "" {
		t.Errorf("expected empty string, got: %s", text)
	}
}
