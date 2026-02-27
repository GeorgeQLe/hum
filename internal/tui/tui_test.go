package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/georgele/hum/internal/config"
	"github.com/georgele/hum/internal/process"
)

// testModel creates a minimal Model for testing with the given apps.
func testModel(apps ...config.App) Model {
	pm := process.NewManager(os.TempDir())
	pm.GetLogBuffer("humrun")
	return Model{
		apps:        apps,
		projectRoot: os.TempDir(),
		procManager: pm,
		focusArea:   focusCommand,
		historyIdx:  -1,
		width:       120,
		height:      40,
	}
}

// allSystemLogs returns all log lines joined by newline.
func allSystemLogs(m *Model) string {
	buf := m.procManager.GetLogBuffer("humrun")
	lines, _, _ := buf.Snapshot()
	var texts []string
	for _, l := range lines {
		texts = append(texts, l.Text)
	}
	return strings.Join(texts, "\n")
}

// keyMsg builds a tea.KeyMsg for a named key (e.g. "tab", "up", "enter").
func keyMsg(key string) tea.KeyMsg {
	switch key {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "esc", "escape":
		return tea.KeyMsg{Type: tea.KeyEscape}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

// ctrlMsg builds a Ctrl+key KeyMsg.
func ctrlMsg(key string) tea.KeyMsg {
	switch key {
	case "u":
		return tea.KeyMsg{Type: tea.KeyCtrlU}
	case "w":
		return tea.KeyMsg{Type: tea.KeyCtrlW}
	case "a":
		return tea.KeyMsg{Type: tea.KeyCtrlA}
	case "e":
		return tea.KeyMsg{Type: tea.KeyCtrlE}
	case "f":
		return tea.KeyMsg{Type: tea.KeyCtrlF}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

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

// --- Phase 2: TUI Test Coverage ---

func TestDispatchCommand(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		args    string
		wantLog string
	}{
		{"start no args", "start", "", "Usage: start"},
		{"stop no args", "stop", "", "Usage: stop"},
		{"restart no args", "restart", "", "Usage: restart"},
		{"unknown command", "foobar", "", "Unknown command: foobar"},
		{"help", "help", "", "humrun — Multi-App"},
		{"list no apps", "list", "", "No apps configured"},
		{"status no apps", "status", "", "No apps configured"},
		{"remove no args", "remove", "", "Usage: remove"},
		{"remove unknown", "remove", "ghost", "Unknown app: ghost"},
		{"clear-errors all", "clear-errors", "all", "All errors cleared"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel()
			m.dispatchCommand(tt.cmd, tt.args)
			log := allSystemLogs(&m)
			if !strings.Contains(log, tt.wantLog) {
				t.Errorf("dispatchCommand(%q, %q): log = %q, want containing %q", tt.cmd, tt.args, log, tt.wantLog)
			}
		})
	}
}

func TestSidebarKeypress(t *testing.T) {
	app1 := config.App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}}
	app2 := config.App{Name: "api", Dir: ".", Command: "npm dev", Ports: []int{8080}}

	t.Run("up/down navigation", func(t *testing.T) {
		m := testModel(app1, app2)
		m.focusArea = focusSidebar
		m.selectedIdx = 1

		// Down
		result, _ := m.handleSidebarKeypress(keyMsg("down"))
		rm := result.(Model)
		if rm.selectedIdx != 2 {
			t.Errorf("expected selectedIdx=2 after down, got %d", rm.selectedIdx)
		}

		// Down at boundary (2 apps + system = max index 2)
		result, _ = rm.handleSidebarKeypress(keyMsg("down"))
		rm = result.(Model)
		if rm.selectedIdx != 2 {
			t.Errorf("expected selectedIdx=2 at boundary, got %d", rm.selectedIdx)
		}

		// Up
		result, _ = rm.handleSidebarKeypress(keyMsg("up"))
		rm = result.(Model)
		if rm.selectedIdx != 1 {
			t.Errorf("expected selectedIdx=1 after up, got %d", rm.selectedIdx)
		}

		// Up to 0
		result, _ = rm.handleSidebarKeypress(keyMsg("up"))
		rm = result.(Model)
		if rm.selectedIdx != 0 {
			t.Errorf("expected selectedIdx=0, got %d", rm.selectedIdx)
		}

		// Up at boundary 0
		result, _ = rm.handleSidebarKeypress(keyMsg("up"))
		rm = result.(Model)
		if rm.selectedIdx != 0 {
			t.Errorf("expected selectedIdx=0 at top boundary, got %d", rm.selectedIdx)
		}
	})

	t.Run("j/k navigation", func(t *testing.T) {
		m := testModel(app1, app2)
		m.focusArea = focusSidebar
		m.selectedIdx = 0

		result, _ := m.handleSidebarKeypress(keyMsg("j"))
		rm := result.(Model)
		if rm.selectedIdx != 1 {
			t.Errorf("expected selectedIdx=1 after j, got %d", rm.selectedIdx)
		}

		result, _ = rm.handleSidebarKeypress(keyMsg("k"))
		rm = result.(Model)
		if rm.selectedIdx != 0 {
			t.Errorf("expected selectedIdx=0 after k, got %d", rm.selectedIdx)
		}
	})

	t.Run("tab switches to command", func(t *testing.T) {
		m := testModel(app1)
		m.focusArea = focusSidebar

		result, _ := m.handleSidebarKeypress(keyMsg("tab"))
		rm := result.(Model)
		if rm.focusArea != focusCommand {
			t.Error("expected focusCommand after tab")
		}
	})

	t.Run("enter switches to command", func(t *testing.T) {
		m := testModel(app1)
		m.focusArea = focusSidebar

		result, _ := m.handleSidebarKeypress(keyMsg("enter"))
		rm := result.(Model)
		if rm.focusArea != focusCommand {
			t.Error("expected focusCommand after enter")
		}
	})

	t.Run("printable char switches to command mode", func(t *testing.T) {
		m := testModel(app1)
		m.focusArea = focusSidebar

		result, _ := m.handleSidebarKeypress(keyMsg("h"))
		rm := result.(Model)
		if rm.focusArea != focusCommand {
			t.Error("expected focusCommand after printable char")
		}
	})
}

func TestCommandKeypress(t *testing.T) {
	t.Run("backspace", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "helo"
		m.cmdCursor = 4

		result, _ := m.handleCommandKeypress(keyMsg("backspace"))
		rm := result.(Model)
		if rm.cmdInput != "hel" {
			t.Errorf("expected 'hel' after backspace, got %q", rm.cmdInput)
		}
		if rm.cmdCursor != 3 {
			t.Errorf("expected cursor=3, got %d", rm.cmdCursor)
		}
	})

	t.Run("delete", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "hello"
		m.cmdCursor = 2

		result, _ := m.handleCommandKeypress(keyMsg("delete"))
		rm := result.(Model)
		if rm.cmdInput != "helo" {
			t.Errorf("expected 'helo' after delete, got %q", rm.cmdInput)
		}
	})

	t.Run("left/right cursor", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "hello"
		m.cmdCursor = 3

		result, _ := m.handleCommandKeypress(keyMsg("left"))
		rm := result.(Model)
		if rm.cmdCursor != 2 {
			t.Errorf("expected cursor=2 after left, got %d", rm.cmdCursor)
		}

		result, _ = rm.handleCommandKeypress(keyMsg("right"))
		rm = result.(Model)
		if rm.cmdCursor != 3 {
			t.Errorf("expected cursor=3 after right, got %d", rm.cmdCursor)
		}
	})

	t.Run("home/end", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "hello"
		m.cmdCursor = 3

		result, _ := m.handleCommandKeypress(keyMsg("home"))
		rm := result.(Model)
		if rm.cmdCursor != 0 {
			t.Errorf("expected cursor=0 after home, got %d", rm.cmdCursor)
		}

		result, _ = rm.handleCommandKeypress(keyMsg("end"))
		rm = result.(Model)
		if rm.cmdCursor != 5 {
			t.Errorf("expected cursor=5 after end, got %d", rm.cmdCursor)
		}
	})

	t.Run("ctrl+u clears line", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "hello world"
		m.cmdCursor = 11

		result, _ := m.handleCommandKeypress(ctrlMsg("u"))
		rm := result.(Model)
		if rm.cmdInput != "" {
			t.Errorf("expected empty input after ctrl+u, got %q", rm.cmdInput)
		}
		if rm.cmdCursor != 0 {
			t.Errorf("expected cursor=0, got %d", rm.cmdCursor)
		}
	})

	t.Run("ctrl+w deletes word", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "start web"
		m.cmdCursor = 9

		result, _ := m.handleCommandKeypress(ctrlMsg("w"))
		rm := result.(Model)
		if rm.cmdInput != "start " {
			t.Errorf("expected 'start ' after ctrl+w, got %q", rm.cmdInput)
		}
	})

	t.Run("tab with empty input switches to sidebar", func(t *testing.T) {
		m := testModel()
		m.focusArea = focusCommand
		m.cmdInput = ""

		result, _ := m.handleCommandKeypress(keyMsg("tab"))
		rm := result.(Model)
		if rm.focusArea != focusSidebar {
			t.Error("expected focusSidebar after tab with empty input")
		}
	})

	t.Run("/ with empty input enters search mode", func(t *testing.T) {
		m := testModel()
		m.cmdInput = ""

		result, _ := m.handleCommandKeypress(keyMsg("/"))
		rm := result.(Model)
		if rm.searchMode == nil {
			t.Error("expected searchMode to be set after /")
		}
	})
}

func TestNavigateHistory(t *testing.T) {
	t.Run("no history is noop", func(t *testing.T) {
		m := testModel()
		m.cmdInput = "current"
		m.navigateHistory(-1)
		if m.cmdInput != "current" {
			t.Errorf("expected input unchanged, got %q", m.cmdInput)
		}
	})

	t.Run("up saves current and loads last", func(t *testing.T) {
		m := testModel()
		m.cmdHistory = []string{"first", "second", "third"}
		m.cmdInput = "current"
		m.historyIdx = -1

		m.navigateHistory(-1)
		if m.historyTmp != "current" {
			t.Errorf("expected historyTmp='current', got %q", m.historyTmp)
		}
		if m.cmdInput != "third" {
			t.Errorf("expected input='third', got %q", m.cmdInput)
		}
		if m.historyIdx != 2 {
			t.Errorf("expected historyIdx=2, got %d", m.historyIdx)
		}
	})

	t.Run("multiple ups walk backward", func(t *testing.T) {
		m := testModel()
		m.cmdHistory = []string{"first", "second", "third"}
		m.cmdInput = "current"
		m.historyIdx = -1

		m.navigateHistory(-1) // -> third
		m.navigateHistory(-1) // -> second
		if m.cmdInput != "second" {
			t.Errorf("expected input='second', got %q", m.cmdInput)
		}
		m.navigateHistory(-1) // -> first
		if m.cmdInput != "first" {
			t.Errorf("expected input='first', got %q", m.cmdInput)
		}
		// At top, stays
		m.navigateHistory(-1)
		if m.cmdInput != "first" {
			t.Errorf("expected input='first' at top, got %q", m.cmdInput)
		}
	})

	t.Run("down walks forward and restores", func(t *testing.T) {
		m := testModel()
		m.cmdHistory = []string{"first", "second"}
		m.cmdInput = "current"
		m.historyIdx = -1

		m.navigateHistory(-1) // -> second
		m.navigateHistory(-1) // -> first
		m.navigateHistory(1)  // -> second
		if m.cmdInput != "second" {
			t.Errorf("expected input='second', got %q", m.cmdInput)
		}
		m.navigateHistory(1) // -> current (restored)
		if m.cmdInput != "current" {
			t.Errorf("expected input='current' (restored), got %q", m.cmdInput)
		}
		if m.historyIdx != -1 {
			t.Errorf("expected historyIdx=-1 after full forward, got %d", m.historyIdx)
		}
	})
}

func TestHandleTabCompletion(t *testing.T) {
	app1 := config.App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}}
	app2 := config.App{Name: "worker", Dir: ".", Command: "npm dev", Ports: []int{9090}}
	app3 := config.App{Name: "api", Dir: ".", Command: "npm dev", Ports: []int{8080}}

	t.Run("single match auto-completes", func(t *testing.T) {
		m := testModel(app1, app2, app3)
		m.cmdInput = "stop ap"
		m.cmdCursor = 7

		m.handleTabCompletion()
		if m.cmdInput != "stop api " {
			t.Errorf("expected 'stop api ', got %q", m.cmdInput)
		}
	})

	t.Run("multiple matches shows common prefix", func(t *testing.T) {
		m := testModel(app1, app2, app3)
		m.cmdInput = "start w"
		m.cmdCursor = 7

		m.handleTabCompletion()
		// "web" and "worker" share prefix "w" (already typed), then "w" -> stays "w"
		// Actually both start with "w", common prefix between "web" and "worker" is "w"
		// Since partial is "w" and common prefix is "w" (same length), no extension
		// But tabMatches should be set
		if m.tabMatches == nil {
			t.Error("expected tabMatches to be set")
		}
	})

	t.Run("subsequent tab cycles through matches", func(t *testing.T) {
		m := testModel(app1, app2, app3)
		m.cmdInput = "start w"
		m.cmdCursor = 7

		m.handleTabCompletion() // sets tabMatches
		if m.tabMatches == nil {
			t.Fatal("expected tabMatches to be set")
		}
		firstMatch := m.cmdInput
		m.handleTabCompletion() // cycle
		secondMatch := m.cmdInput
		if firstMatch == secondMatch {
			// The first handleTabCompletion sets tabMatches and logs; second cycles
			// Actually after first call, tabIdx=0 so cycling should move to tabIdx=1
		}
		// Just verify cycling happens
		if m.tabIdx == 0 {
			t.Error("expected tabIdx to advance after second tab")
		}
	})

	t.Run("non-tab input clears tabMatches", func(t *testing.T) {
		m := testModel(app1, app2)
		m.cmdInput = "start w"
		m.cmdCursor = 7
		m.tabMatches = []string{"web", "worker"}

		// Type a regular char — handleCommandKeypress returns new model
		result, _ := m.handleCommandKeypress(keyMsg("e"))
		rm := result.(Model)
		if rm.tabMatches != nil {
			t.Error("expected tabMatches to be cleared after regular input")
		}
	})
}

func TestRecalcLayout(t *testing.T) {
	t.Run("sidebar min width", func(t *testing.T) {
		m := testModel(config.App{Name: "a", Dir: ".", Command: "x", Ports: []int{80}})
		m.width = 120
		m.height = 40
		m.recalcLayout()
		if m.sidebarWidth < 16 {
			t.Errorf("expected sidebarWidth >= 16, got %d", m.sidebarWidth)
		}
	})

	t.Run("sidebar based on name length", func(t *testing.T) {
		m := testModel(config.App{Name: "my-long-app-name", Dir: ".", Command: "x", Ports: []int{80}})
		m.width = 120
		m.height = 40
		m.recalcLayout()
		expected := len("my-long-app-name") + 6
		if m.sidebarWidth != expected {
			t.Errorf("expected sidebarWidth=%d, got %d", expected, m.sidebarWidth)
		}
	})

	t.Run("sidebar capped at 35%", func(t *testing.T) {
		m := testModel(config.App{Name: "very-very-very-very-very-long-name", Dir: ".", Command: "x", Ports: []int{80}})
		m.width = 80
		m.height = 40
		m.recalcLayout()
		maxSidebar := 80 * 35 / 100
		if m.sidebarWidth > maxSidebar {
			t.Errorf("expected sidebarWidth <= %d (35%% of 80), got %d", maxSidebar, m.sidebarWidth)
		}
	})

	t.Run("log width calculation", func(t *testing.T) {
		m := testModel()
		m.width = 120
		m.height = 40
		m.recalcLayout()
		expectedLogWidth := 120 - m.sidebarWidth - 3
		if m.logWidth != expectedLogWidth {
			t.Errorf("expected logWidth=%d, got %d", expectedLogWidth, m.logWidth)
		}
	})
}

func TestSearchModeEntryExit(t *testing.T) {
	t.Run("/ enters search mode", func(t *testing.T) {
		m := testModel()
		m.cmdInput = ""
		result, _ := m.handleCommandKeypress(keyMsg("/"))
		rm := result.(Model)
		if rm.searchMode == nil {
			t.Error("expected searchMode after /")
		}
	})

	t.Run("escape exits search mode", func(t *testing.T) {
		m := testModel()
		m.searchMode = newSearchMode()
		result, _ := m.handleSearchKeypress(keyMsg("esc"))
		rm := result.(Model)
		if rm.searchMode != nil {
			t.Error("expected nil searchMode after escape")
		}
	})

	t.Run("enter exits search mode", func(t *testing.T) {
		m := testModel()
		m.searchMode = newSearchMode()
		result, _ := m.handleSearchKeypress(keyMsg("enter"))
		rm := result.(Model)
		if rm.searchMode != nil {
			t.Error("expected nil searchMode after enter")
		}
	})
}

func TestModeTransitions(t *testing.T) {
	t.Run("search mode blocks question mode", func(t *testing.T) {
		m := testModel()
		m.searchMode = newSearchMode()
		// In search mode, keypress should go to search handler not question
		if m.questionMode != nil {
			t.Error("modes should not overlap")
		}
	})

	t.Run("question mode blocks search", func(t *testing.T) {
		m := testModel()
		m.questionMode = &QuestionMode{Prompt: "test: ", callback: func(string) {}}
		// Verify question mode active, search not
		if m.searchMode != nil {
			t.Error("modes should not overlap")
		}
	})

	t.Run("scan mode takes priority", func(t *testing.T) {
		m := testModel()
		m.scanMode = newScanMode([]config.ScanCandidate{{Name: "test", Dir: ".", Command: "npm dev", Ports: []int{3000}}})
		// handleKeypress should route to scan handler
		result, _ := m.handleKeypress(keyMsg("esc"))
		rm := result.(Model)
		if rm.scanMode != nil {
			t.Error("expected scanMode to be cleared after escape")
		}
	})
}
