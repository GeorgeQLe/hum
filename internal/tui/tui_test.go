package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/process"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArg string
	}{
		{"start web", "start", "web"},
		{"stop all", "stop", "all"},
		{"help", "help", ""},
		{"  restart  api  ", "restart", "api"},
		{"remove my-app", "remove", "my-app"},
		{"status", "status", ""},
		{"start", "start", ""},
	}

	for _, tt := range tests {
		cmd, args := parseCommand(tt.input)
		if cmd != tt.wantCmd {
			t.Errorf("parseCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.wantCmd)
		}
		if args != tt.wantArg {
			t.Errorf("parseCommand(%q) args = %q, want %q", tt.input, args, tt.wantArg)
		}
	}
}

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"start", "stop", "status"}, "st"},
		{[]string{"restart"}, "restart"},
		{[]string{"help", "hello"}, "hel"},
		{[]string{"abc", "def"}, ""},
		{[]string{}, ""},
		{[]string{"same", "same", "same"}, "same"},
	}

	for _, tt := range tests {
		got := commonPrefix(tt.input)
		if got != tt.want {
			t.Errorf("commonPrefix(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestComplete(t *testing.T) {
	m := &Model{
		apps: []config.App{
			{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}},
			{Name: "api", Dir: ".", Command: "npm dev", Ports: []int{8080}},
			{Name: "worker", Dir: ".", Command: "npm dev", Ports: []int{9090}},
		},
	}

	// Command completion
	matches, partial := m.complete("st")
	if partial != "st" {
		t.Errorf("partial = %q, want %q", partial, "st")
	}
	if len(matches) < 2 {
		t.Errorf("expected at least 2 matches for 'st', got %d", len(matches))
	}

	// Argument completion
	matches, partial = m.complete("start w")
	if partial != "w" {
		t.Errorf("partial = %q, want %q", partial, "w")
	}
	// Should match "web" and "worker"
	foundWeb, foundWorker := false, false
	for _, m := range matches {
		if m == "web" {
			foundWeb = true
		}
		if m == "worker" {
			foundWorker = true
		}
	}
	if !foundWeb || !foundWorker {
		t.Errorf("expected web and worker in matches, got %v", matches)
	}

	// "all" completion
	matches, _ = m.complete("stop a")
	foundAll := false
	for _, m := range matches {
		if m == "all" {
			foundAll = true
		}
	}
	if !foundAll {
		t.Error("expected 'all' in stop completions")
	}

	// No matches
	matches, _ = m.complete("xyz")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for 'xyz', got %d", len(matches))
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		dur  time.Duration
		want string
	}{
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3661 * time.Second, "1h 1m"},
		{0, "0s"},
		{30 * time.Second, "30s"},
		{7200 * time.Second, "2h 0m"},
	}

	for _, tt := range tests {
		got := formatUptime(tt.dur)
		if got != tt.want {
			t.Errorf("formatUptime(%v) = %q, want %q", tt.dur, got, tt.want)
		}
	}
}

func TestTruncateToWidth(t *testing.T) {
	tests := []struct {
		input string
		width int
		check func(string) bool
		desc  string
	}{
		{
			input: "hello",
			width: 10,
			check: func(s string) bool { return len(s) == 10 && strings.HasPrefix(s, "hello") },
			desc:  "should pad short string",
		},
		{
			input: "hello world long text",
			width: 10,
			check: func(s string) bool {
				stripped := process.StripAnsi(s)
				return len(stripped) <= 11 // 10 chars + possible reset
			},
			desc: "should truncate long string",
		},
		{
			input: "",
			width: 0,
			check: func(s string) bool { return s == "" },
			desc:  "zero width returns empty",
		},
		{
			input: "\x1b[31mred text\x1b[0m",
			width: 20,
			check: func(s string) bool {
				return strings.Contains(s, "\x1b[31m") // ANSI should be preserved
			},
			desc: "should preserve ANSI when not truncating",
		},
		{
			input: "\x1b[31mred text that is very long\x1b[0m",
			width: 8,
			check: func(s string) bool {
				return strings.Contains(s, "\x1b[31m") // ANSI should be preserved even when truncating
			},
			desc: "should preserve ANSI when truncating",
		},
	}

	for _, tt := range tests {
		got := truncateToWidth(tt.input, tt.width)
		if !tt.check(got) {
			t.Errorf("truncateToWidth(%q, %d): %s — got %q", tt.input, tt.width, tt.desc, got)
		}
	}
}

func TestSearchModeUpdateMatches(t *testing.T) {
	logBuf := process.NewLogBuffer()
	logBuf.Append("line one hello\n", false)
	logBuf.Append("line two world\n", false)
	logBuf.Append("line three hello world\n", false)

	sm := newSearchMode()
	sm.pattern = "hello"
	sm.updateMatches(logBuf)

	if len(sm.matches) != 2 {
		t.Errorf("expected 2 matches for 'hello', got %d", len(sm.matches))
	}
	if sm.matchIdx != 0 {
		t.Errorf("expected matchIdx 0, got %d", sm.matchIdx)
	}

	// Update with new pattern
	sm.pattern = "world"
	sm.updateMatches(logBuf)

	if len(sm.matches) != 2 {
		t.Errorf("expected 2 matches for 'world', got %d", len(sm.matches))
	}

	// No matches
	sm.pattern = "nonexistent"
	sm.updateMatches(logBuf)
	if len(sm.matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(sm.matches))
	}
	if sm.matchIdx != -1 {
		t.Errorf("expected matchIdx -1, got %d", sm.matchIdx)
	}
}

func TestSearchModeNavigate(t *testing.T) {
	sm := newSearchMode()
	sm.matches = []SearchMatch{
		{LineIdx: 0, Start: 0, End: 5},
		{LineIdx: 1, Start: 0, End: 5},
		{LineIdx: 2, Start: 0, End: 5},
	}
	sm.matchIdx = 0

	// Forward
	sm.navigate(1)
	if sm.matchIdx != 1 {
		t.Errorf("expected matchIdx 1, got %d", sm.matchIdx)
	}

	// Forward wrap
	sm.matchIdx = 2
	sm.navigate(1)
	if sm.matchIdx != 0 {
		t.Errorf("expected matchIdx 0 after wrap, got %d", sm.matchIdx)
	}

	// Backward wrap
	sm.matchIdx = 0
	sm.navigate(-1)
	if sm.matchIdx != 2 {
		t.Errorf("expected matchIdx 2 after backward wrap, got %d", sm.matchIdx)
	}
}

func TestSearchModeCurrentMatch(t *testing.T) {
	sm := newSearchMode()

	// No matches
	if sm.currentMatch() != nil {
		t.Error("expected nil match when no matches")
	}

	sm.matches = []SearchMatch{{LineIdx: 5, Start: 0, End: 3}}
	sm.matchIdx = 0

	m := sm.currentMatch()
	if m == nil {
		t.Fatal("expected non-nil match")
	}
	if m.LineIdx != 5 {
		t.Errorf("expected LineIdx 5, got %d", m.LineIdx)
	}
}

func TestSearchModeRegexFallback(t *testing.T) {
	logBuf := process.NewLogBuffer()
	logBuf.Append("test [xyz123] content\n", false)

	sm := newSearchMode()
	sm.pattern = "[xyz123]" // valid regex that matches individual chars, not same as literal
	sm.updateMatches(logBuf)

	// "[xyz123]" as regex matches individual chars x, y, z, 1, 2, 3
	// It's a valid regex so won't fall back to literal
	if len(sm.matches) == 0 {
		t.Error("expected matches for regex '[xyz123]'")
	}

	// Now test actual invalid regex that forces fallback
	sm2 := newSearchMode()
	sm2.pattern = "[unclosed" // invalid regex
	sm2.updateMatches(logBuf)
	// Should fall back to literal search for "[unclosed"
	if len(sm2.matches) != 0 {
		t.Errorf("expected 0 matches for literal '[unclosed', got %d", len(sm2.matches))
	}

	logBuf2 := process.NewLogBuffer()
	logBuf2.Append("has [unclosed in it\n", false)
	sm2.updateMatches(logBuf2)
	if len(sm2.matches) != 1 {
		t.Errorf("expected 1 match for literal '[unclosed' in matching text, got %d", len(sm2.matches))
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		input string
		width int
		want  int // expected visual length
	}{
		{"hello", 10, 10},
		{"hello", 3, 5}, // doesn't truncate, returns as-is
		{"", 5, 5},
	}

	for _, tt := range tests {
		got := padRight(tt.input, tt.width)
		vl := visLen(got)
		if vl != tt.want {
			t.Errorf("padRight(%q, %d) visual len = %d, want %d", tt.input, tt.width, vl, tt.want)
		}
	}
}

func TestVisLen(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"\x1b[31mhello\x1b[0m", 5},
		{"", 0},
	}

	for _, tt := range tests {
		got := visLen(tt.input)
		if got != tt.want {
			t.Errorf("visLen(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
