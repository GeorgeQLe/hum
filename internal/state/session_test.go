package state

import (
	"testing"
)

func TestSaveLoadSession(t *testing.T) {
	tmpDir := t.TempDir()

	apps := []string{"web", "api", "worker"}
	err := SaveSession(tmpDir, apps)
	if err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	loaded := LoadSession(tmpDir)
	if len(loaded) != len(apps) {
		t.Fatalf("LoadSession() returned %d apps, want %d", len(loaded), len(apps))
	}
	for i, name := range loaded {
		if name != apps[i] {
			t.Errorf("loaded[%d] = %q, want %q", i, name, apps[i])
		}
	}
}

func TestLoadSessionNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	loaded := LoadSession(tmpDir)
	if loaded != nil {
		t.Errorf("LoadSession() returned %v for nonexistent file, want nil", loaded)
	}
}

func TestClearSession(t *testing.T) {
	tmpDir := t.TempDir()

	SaveSession(tmpDir, []string{"web"})
	ClearSession(tmpDir)

	loaded := LoadSession(tmpDir)
	if loaded != nil {
		t.Errorf("LoadSession() returned %v after ClearSession, want nil", loaded)
	}
}
