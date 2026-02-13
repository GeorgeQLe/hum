package process

import "testing"

func TestV8ErrorParser(t *testing.T) {
	lines := []string{
		"TypeError: undefined is not a function",
		"    at processQueue (src/app.ts:42:11)",
		"    at Router.handle (src/router.ts:100:5)",
		"    at Object.<anonymous> (src/index.ts:10:3)",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.Kind != ErrorV8 {
		t.Errorf("expected ErrorV8, got %d", parsed.Kind)
	}
	if parsed.ErrorType != "TypeError" {
		t.Errorf("expected ErrorType 'TypeError', got %q", parsed.ErrorType)
	}
	if parsed.Message != "undefined is not a function" {
		t.Errorf("expected message 'undefined is not a function', got %q", parsed.Message)
	}
	if parsed.Location == nil {
		t.Fatal("expected non-nil Location")
	}
	if parsed.Location.File != "src/app.ts" || parsed.Location.Line != 42 || parsed.Location.Column != 11 {
		t.Errorf("expected location src/app.ts:42:11, got %s", parsed.Location.String())
	}
	if len(parsed.StackTrace) != 3 {
		t.Errorf("expected 3 stack frames, got %d", len(parsed.StackTrace))
	}
}

func TestV8ReferenceError(t *testing.T) {
	lines := []string{
		"ReferenceError: myVar is not defined",
		"    at eval (eval at <anonymous>:1:1)",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.ErrorType != "ReferenceError" {
		t.Errorf("expected 'ReferenceError', got %q", parsed.ErrorType)
	}
	if parsed.Message != "myVar is not defined" {
		t.Errorf("expected message, got %q", parsed.Message)
	}
}

func TestTSCompilerParser(t *testing.T) {
	lines := []string{
		"src/components/App.tsx(15,3): error TS2322: Type 'string' is not assignable to type 'number'.",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.Kind != ErrorTSCompiler {
		t.Errorf("expected ErrorTSCompiler, got %d", parsed.Kind)
	}
	if parsed.ErrorType != "TS2322" {
		t.Errorf("expected ErrorType 'TS2322', got %q", parsed.ErrorType)
	}
	if parsed.Message != "Type 'string' is not assignable to type 'number'." {
		t.Errorf("unexpected message: %q", parsed.Message)
	}
	if parsed.Location == nil {
		t.Fatal("expected non-nil Location")
	}
	if parsed.Location.File != "src/components/App.tsx" {
		t.Errorf("expected file 'src/components/App.tsx', got %q", parsed.Location.File)
	}
	if parsed.Location.Line != 15 || parsed.Location.Column != 3 {
		t.Errorf("expected 15:3, got %d:%d", parsed.Location.Line, parsed.Location.Column)
	}
}

func TestBundlerViteParser(t *testing.T) {
	lines := []string{
		"[vite] Internal server error: Transform failed",
		"  Plugin: vite:esbuild",
		"  File: /src/main.ts:10:5",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.Kind != ErrorBundler {
		t.Errorf("expected ErrorBundler, got %d", parsed.Kind)
	}
	if parsed.ErrorType != "Vite Error" {
		t.Errorf("expected 'Vite Error', got %q", parsed.ErrorType)
	}
}

func TestBundlerEsbuildParser(t *testing.T) {
	lines := []string{
		"✘ [ERROR] Could not resolve \"missing-module\"",
		"",
		"    src/index.ts:1:0:",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.Kind != ErrorBundler {
		t.Errorf("expected ErrorBundler, got %d", parsed.Kind)
	}
	if parsed.ErrorType != "esbuild Error" {
		t.Errorf("expected 'esbuild Error', got %q", parsed.ErrorType)
	}
}

func TestBundlerWebpackParser(t *testing.T) {
	lines := []string{
		"ERROR in ./src/index.js",
		"Module not found: Can't resolve 'missing'",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.Kind != ErrorBundler {
		t.Errorf("expected ErrorBundler, got %d", parsed.Kind)
	}
	if parsed.ErrorType != "webpack Error" {
		t.Errorf("expected 'webpack Error', got %q", parsed.ErrorType)
	}
}

func TestGenericParser(t *testing.T) {
	lines := []string{
		"Something failed completely",
		"  at /path/to/file.js:10:5",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.Kind != ErrorGeneric {
		t.Errorf("expected ErrorGeneric, got %d", parsed.Kind)
	}
	if parsed.Message != "Something failed completely" {
		t.Errorf("expected message, got %q", parsed.Message)
	}
}

func TestParsedErrorDedupKey(t *testing.T) {
	pe := &ParsedError{
		ErrorType: "TypeError",
		Message:   "undefined is not a function",
	}
	key := pe.DedupKey()
	expected := "TypeError:undefined is not a function"
	if key != expected {
		t.Errorf("expected dedup key %q, got %q", expected, key)
	}
}

func TestParserWithAnsi(t *testing.T) {
	lines := []string{
		"\x1b[31mTypeError: Cannot read property 'x' of null\x1b[0m",
		"\x1b[90m    at foo (/app/src/bar.ts:5:10)\x1b[0m",
	}

	parsed := ParseError(lines)
	if parsed == nil {
		t.Fatal("expected non-nil ParsedError")
	}
	if parsed.ErrorType != "TypeError" {
		t.Errorf("expected 'TypeError', got %q", parsed.ErrorType)
	}
	if parsed.Location == nil {
		t.Fatal("expected location")
	}
	if parsed.Location.File != "/app/src/bar.ts" {
		t.Errorf("expected file '/app/src/bar.ts', got %q", parsed.Location.File)
	}
}
