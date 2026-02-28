package process

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLogBufferLineTruncation(t *testing.T) {
	buf := NewLogBuffer()
	longLine := strings.Repeat("A", MaxLineLength+1000)
	buf.Append(longLine, false)

	maxExpected := MaxLineLength + len("... [truncated]")
	line, ok := buf.GetLine(0)
	if !ok {
		t.Fatal("expected at least one line in buffer")
	}
	if len(line.Text) > maxExpected {
		t.Errorf("line length %d exceeds maximum expected %d", len(line.Text), maxExpected)
	}
}

func TestLogBufferMemoryBound(t *testing.T) {
	buf := NewLogBuffer()
	// Fill buffer with MaxLogLines lines of maximum-length content
	for i := 0; i < MaxLogLines; i++ {
		line := strings.Repeat("X", MaxLineLength)
		buf.Append(line+"\n", false)
	}

	count := buf.LineCount()
	if count > MaxLogLines {
		t.Errorf("line count %d exceeds MaxLogLines %d", count, MaxLogLines)
	}
}

func TestLogBufferDoesNotGrowUnbounded(t *testing.T) {
	buf := NewLogBuffer()
	for i := 0; i < 10000; i++ {
		buf.Append("line\n", false)
	}

	count := buf.LineCount()
	if count != MaxLogLines {
		t.Errorf("expected LineCount() == %d, got %d", MaxLogLines, count)
	}
}

func TestFilteredEnvStripsSecrets(t *testing.T) {
	os.Setenv("HUMSAFE_SECRET_KEY", "supersecret")
	os.Setenv("HUMRUN_TOKEN", "mytoken")
	defer os.Unsetenv("HUMSAFE_SECRET_KEY")
	defer os.Unsetenv("HUMRUN_TOKEN")

	env := FilteredEnv()
	for _, e := range env {
		if strings.HasPrefix(e, "HUMSAFE_") {
			t.Errorf("FilteredEnv should strip HUMSAFE_ vars, found: %s", e)
		}
		if strings.HasPrefix(e, "HUMRUN_TOKEN") {
			t.Errorf("FilteredEnv should strip HUMRUN_TOKEN vars, found: %s", e)
		}
	}
}

func TestFilteredEnvKeepsNormal(t *testing.T) {
	env := FilteredEnv()
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			found = true
			break
		}
	}
	if !found {
		t.Error("FilteredEnv should include normal env vars like PATH")
	}
}

func TestSendEventDoesNotBlock(t *testing.T) {
	m := NewManager(t.TempDir())

	// Fill the event channel to capacity
	for i := 0; i < 8192; i++ {
		m.eventCh <- ProcessEvent{Type: EventOutput, AppName: "test"}
	}

	done := make(chan struct{})
	go func() {
		m.sendEvent(ProcessEvent{Type: EventOutput, AppName: "test"})
		close(done)
	}()

	select {
	case <-done:
		// sendEvent returned without blocking — success
	case <-time.After(3 * time.Second):
		t.Fatal("sendEvent blocked on a full channel")
	}
}

func TestGetProcessCommandEmptyOnInvalidPID(t *testing.T) {
	result := getProcessCommand(999999999)
	if result != "" {
		t.Errorf("expected empty string for invalid PID, got %q", result)
	}
}
