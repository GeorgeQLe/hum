package process

import "testing"

func TestErrorDetector_TruePositives(t *testing.T) {
	d := NewErrorDetector()
	positives := []string{
		"ERROR: something went wrong",
		"Error: file not found",
		"Exception in thread \"main\" java.lang.NullPointerException",
		"Build Failed",
		"FATAL error in module",
		"TypeError: undefined is not a function",
		"at Module._compile (/path/file.js:10:20)",
		"    at Object.<anonymous>",
		"ReferenceError: x is not defined",
		"SyntaxError: Unexpected token",
	}
	for _, line := range positives {
		if !d.IsError(line) {
			t.Errorf("expected IsError(%q) = true", line)
		}
	}
}

func TestErrorDetector_TrueNegatives(t *testing.T) {
	d := NewErrorDetector()
	negatives := []string{
		"this is a normal log line",
		"Starting server on port 3000",
		"Compilation successful",
		"",
		"Listening on http://localhost:8080",
	}
	for _, line := range negatives {
		if d.IsError(line) {
			t.Errorf("expected IsError(%q) = false", line)
		}
	}
}

func TestErrorDetector_Exclusions(t *testing.T) {
	d := NewErrorDetector()
	excluded := []string{
		"0 errors",
		"Found 0 errors. Watching for file changes.",
		"error handling module loaded",
		"errorHandler initialized",
		"Failed to find cached package — downloading",
		"error-free deployment",
		"no errors found",
		"No Errors detected",
		"if error != nil {",
		"onError callback registered",
		"handleError function called",
		"warning: 'foo' is deprecated",
		"Warning: deprecated feature used",
	}
	for _, line := range excluded {
		if d.IsError(line) {
			t.Errorf("expected IsError(%q) = false (should be excluded)", line)
		}
	}
}

func TestErrorDetector_RedAnsi(t *testing.T) {
	d := NewErrorDetector()
	// Red ANSI should trigger error detection
	redLine := "\x1b[31mSomething went wrong\x1b[0m"
	if !d.IsError(redLine) {
		t.Error("expected red ANSI line to be detected as error")
	}

	// Red ANSI with exclusion content should be excluded
	redExcluded := "\x1b[31m0 errors\x1b[0m"
	if d.IsError(redExcluded) {
		t.Error("expected red ANSI line with excluded content to not be detected")
	}
}

func TestErrorDetector_BackwardCompatibility(t *testing.T) {
	// Ensure the old MatchesErrorPattern still works
	if !MatchesErrorPattern("Error: test") {
		t.Error("MatchesErrorPattern should still detect errors")
	}
	if MatchesErrorPattern("normal line") {
		t.Error("MatchesErrorPattern should not flag normal lines")
	}

	// Ensure IsErrorLine still works
	if !IsErrorLine("Error: test") {
		t.Error("IsErrorLine should still detect errors")
	}
}
