package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "apps.json")
	os.WriteFile(configPath, []byte("[]"), 0644)

	w, err := NewWatcher(configPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()
	w.Start()

	// Wait a moment for watcher to be ready
	time.Sleep(50 * time.Millisecond)

	// Trigger a change
	os.WriteFile(configPath, []byte("[]\n"), 0644)

	select {
	case <-w.Changes():
		// Good, change detected
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for change notification")
	}
}

func TestWatcherDebounce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "apps.json")
	os.WriteFile(configPath, []byte("[]"), 0644)

	w, err := NewWatcher(configPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()
	w.Start()

	time.Sleep(50 * time.Millisecond)

	// Trigger multiple rapid changes
	for i := 0; i < 5; i++ {
		os.WriteFile(configPath, []byte("[]\n"), 0644)
		time.Sleep(10 * time.Millisecond)
	}

	// Should get exactly one notification due to debounce
	select {
	case <-w.Changes():
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for change notification")
	}

	// Should not get another notification quickly
	select {
	case <-w.Changes():
		t.Fatal("unexpected second notification")
	case <-time.After(300 * time.Millisecond):
		// Good, no duplicate
	}
}

func TestWatcherIgnoreNext(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "apps.json")
	os.WriteFile(configPath, []byte("[]"), 0644)

	w, err := NewWatcher(configPath)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Stop()
	w.Start()

	time.Sleep(50 * time.Millisecond)

	// Set ignore flag before change
	w.SetIgnoreNext()
	os.WriteFile(configPath, []byte("[]\n"), 0644)

	// Should not get notification
	select {
	case <-w.Changes():
		t.Fatal("should not have received notification after SetIgnoreNext")
	case <-time.After(500 * time.Millisecond):
		// Good, change was ignored
	}
}
