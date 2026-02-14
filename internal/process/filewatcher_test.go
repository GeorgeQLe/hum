package process

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/georgele/devctl/internal/config"
)

func waitEvent(ch <-chan FileWatchEvent, timeout time.Duration) *FileWatchEvent {
	select {
	case evt := <-ch:
		return &evt
	case <-time.After(timeout):
		return nil
	}
}

func TestFileWatchDetectsChange(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Wait for watcher to be ready
	time.Sleep(50 * time.Millisecond)

	// Write a file
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	evt := waitEvent(fm.Events(), 2*time.Second)
	if evt == nil {
		t.Fatal("expected file watch event, got none")
	}
	if evt.AppName != "test-app" {
		t.Errorf("expected appName 'test-app', got %q", evt.AppName)
	}
}

func TestFileWatchDebounce(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Rapid writes
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("v"+string(rune('0'+i))), 0644)
		time.Sleep(20 * time.Millisecond)
	}

	// Should get exactly one event after debounce
	evt := waitEvent(fm.Events(), 2*time.Second)
	if evt == nil {
		t.Fatal("expected one event after debounce")
	}

	// Should not get another event
	evt2 := waitEvent(fm.Events(), 500*time.Millisecond)
	if evt2 != nil {
		t.Error("expected no second event after debounce")
	}
}

func TestFileWatchExtensionFilter(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{Extensions: []string{".go"}}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Write a .txt file — should be ignored
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644)

	evt := waitEvent(fm.Events(), 500*time.Millisecond)
	if evt != nil {
		t.Error("expected no event for .txt file with .go extension filter")
	}

	// Write a .go file — should trigger
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)

	evt = waitEvent(fm.Events(), 2*time.Second)
	if evt == nil {
		t.Fatal("expected event for .go file")
	}
}

func TestFileWatchIgnoreDir(t *testing.T) {
	dir := t.TempDir()
	nm := filepath.Join(dir, "node_modules")
	os.MkdirAll(nm, 0755)

	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Write in node_modules — should be ignored
	os.WriteFile(filepath.Join(nm, "foo.js"), []byte("module.exports = {}"), 0644)

	evt := waitEvent(fm.Events(), 500*time.Millisecond)
	if evt != nil {
		t.Error("expected no event for file in node_modules")
	}
}

func TestFileWatchToggle(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Disable watching
	fm.SetEnabled("test-app", false)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("disabled"), 0644)

	evt := waitEvent(fm.Events(), 500*time.Millisecond)
	if evt != nil {
		t.Error("expected no event when watching is disabled")
	}

	// Re-enable
	fm.SetEnabled("test-app", true)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("enabled"), 0644)

	evt = waitEvent(fm.Events(), 2*time.Second)
	if evt == nil {
		t.Fatal("expected event after re-enabling watch")
	}
}

func TestFileWatchRestartInFlight(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Set restart in-flight
	fm.SetRestartInFlight("test-app", true)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("restarting"), 0644)

	evt := waitEvent(fm.Events(), 500*time.Millisecond)
	if evt != nil {
		t.Error("expected no event during restart in-flight")
	}

	// Clear
	fm.SetRestartInFlight("test-app", false)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("ready"), 0644)

	evt = waitEvent(fm.Events(), 2*time.Second)
	if evt == nil {
		t.Fatal("expected event after clearing restart in-flight")
	}
}

func TestFileWatchNewDirectory(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()
	defer fm.StopAll()

	cfg := &config.WatchConfig{}
	if err := fm.Register("test-app", dir, cfg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Create a new subdirectory
	subDir := filepath.Join(dir, "src")
	os.MkdirAll(subDir, 0755)

	// Wait for the directory to be picked up
	time.Sleep(200 * time.Millisecond)

	// Write a file in the new directory
	os.WriteFile(filepath.Join(subDir, "app.go"), []byte("package src"), 0644)

	evt := waitEvent(fm.Events(), 2*time.Second)
	if evt == nil {
		t.Fatal("expected event for file in newly created directory")
	}
}

func TestFileWatchStopAll(t *testing.T) {
	dir := t.TempDir()
	fm := NewFileWatchManager()

	cfg := &config.WatchConfig{}
	if err := fm.Register("app1", dir, cfg); err != nil {
		t.Fatalf("Register app1: %v", err)
	}
	if err := fm.Register("app2", dir, cfg); err != nil {
		t.Fatalf("Register app2: %v", err)
	}

	if !fm.HasWatch("app1") {
		t.Error("expected HasWatch for app1")
	}
	if !fm.HasWatch("app2") {
		t.Error("expected HasWatch for app2")
	}

	fm.StopAll()

	if fm.HasWatch("app1") {
		t.Error("expected no watcher for app1 after StopAll")
	}
	if fm.HasWatch("app2") {
		t.Error("expected no watcher for app2 after StopAll")
	}
}
