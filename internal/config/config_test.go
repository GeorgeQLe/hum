package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		app     App
		wantErr bool
	}{
		{
			name:    "valid app",
			app:     App{Name: "web", Dir: ".", Command: "npm run dev", Ports: []int{3000}},
			wantErr: false,
		},
		{
			name:    "missing name",
			app:     App{Dir: ".", Command: "npm run dev", Ports: []int{3000}},
			wantErr: true,
		},
		{
			name:    "missing dir",
			app:     App{Name: "web", Command: "npm run dev", Ports: []int{3000}},
			wantErr: true,
		},
		{
			name:    "missing command",
			app:     App{Name: "web", Dir: ".", Ports: []int{3000}},
			wantErr: true,
		},
		{
			name:    "empty ports",
			app:     App{Name: "web", Dir: ".", Command: "npm run dev", Ports: []int{}},
			wantErr: true,
		},
		{
			name:    "invalid port zero",
			app:     App{Name: "web", Dir: ".", Command: "npm run dev", Ports: []int{0}},
			wantErr: true,
		},
		{
			name:    "invalid port too high",
			app:     App{Name: "web", Dir: ".", Command: "npm run dev", Ports: []int{70000}},
			wantErr: true,
		},
		{
			name:    "multiple valid ports",
			app:     App{Name: "web", Dir: ".", Command: "npm run dev", Ports: []int{3000, 3001}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.app.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadSave(t *testing.T) {
	tmpDir := t.TempDir()

	apps := []App{
		{Name: "web", Dir: "packages/web", Command: "pnpm dev", Ports: []int{3000}},
		{Name: "api", Dir: "packages/api", Command: "npm run dev", Ports: []int{8080, 8081}},
	}

	// Save
	err := Save(tmpDir, apps)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, "apps.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("apps.json was not created")
	}

	// Load
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded) != len(apps) {
		t.Fatalf("Load() returned %d apps, want %d", len(loaded), len(apps))
	}

	for i, app := range loaded {
		if app.Name != apps[i].Name {
			t.Errorf("app[%d].Name = %q, want %q", i, app.Name, apps[i].Name)
		}
		if app.Dir != apps[i].Dir {
			t.Errorf("app[%d].Dir = %q, want %q", i, app.Dir, apps[i].Dir)
		}
		if app.Command != apps[i].Command {
			t.Errorf("app[%d].Command = %q, want %q", i, app.Command, apps[i].Command)
		}
		if len(app.Ports) != len(apps[i].Ports) {
			t.Errorf("app[%d].Ports = %v, want %v", i, app.Ports, apps[i].Ports)
		}
	}
}

func TestLoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	apps, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("Load() returned %d apps for nonexistent file, want 0", len(apps))
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "apps.json")
	os.WriteFile(configPath, []byte("not json"), 0644)

	apps, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("Load() returned %d apps for invalid JSON, want 0", len(apps))
	}
}

func TestHasChanged(t *testing.T) {
	base := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}}

	if HasChanged(base, base) {
		t.Error("HasChanged() returned true for identical apps")
	}

	different := App{Name: "web", Dir: "./other", Command: "npm dev", Ports: []int{3000}}
	if !HasChanged(base, different) {
		t.Error("HasChanged() returned false for different dirs")
	}

	differentPorts := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3001}}
	if !HasChanged(base, differentPorts) {
		t.Error("HasChanged() returned false for different ports")
	}

	// Changed command
	differentCmd := App{Name: "web", Dir: ".", Command: "yarn dev", Ports: []int{3000}}
	if !HasChanged(base, differentCmd) {
		t.Error("HasChanged() returned false for different command")
	}

	// Different port count
	morePorts := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000, 3001}}
	if !HasChanged(base, morePorts) {
		t.Error("HasChanged() returned false for different port count")
	}
}

func TestSaveWithOptionalFields(t *testing.T) {
	tmpDir := t.TempDir()

	autoRestart := true
	delay := 2000
	maxRestarts := 3

	apps := []App{
		{
			Name:         "web",
			Dir:          ".",
			Command:      "npm dev",
			Ports:        []int{3000},
			AutoRestart:  &autoRestart,
			RestartDelay: &delay,
			MaxRestarts:  &maxRestarts,
		},
	}

	err := Save(tmpDir, apps)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("expected 1 app, got %d", len(loaded))
	}

	app := loaded[0]
	if app.AutoRestart == nil || !*app.AutoRestart {
		t.Error("expected autoRestart=true")
	}
	if app.RestartDelay == nil || *app.RestartDelay != 2000 {
		t.Error("expected restartDelay=2000")
	}
	if app.MaxRestarts == nil || *app.MaxRestarts != 3 {
		t.Error("expected maxRestarts=3")
	}
}

func TestValidateOptionalFields(t *testing.T) {
	negDelay := -1
	negMax := -1

	app := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, RestartDelay: &negDelay}
	if err := app.Validate(); err == nil {
		t.Error("expected error for negative restartDelay")
	}

	app = App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, MaxRestarts: &negMax}
	if err := app.Validate(); err == nil {
		t.Error("expected error for negative maxRestarts")
	}
}

func TestScanCurrentDir(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name": "test-app", "scripts": {"dev": "next dev"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	result, err := ScanCurrentDir(dir, dir)
	if err != nil {
		t.Fatalf("ScanCurrentDir: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "test-app" {
		t.Errorf("expected name 'test-app', got %q", result.Name)
	}
	if len(result.Ports) == 0 || result.Ports[0] != 3000 {
		t.Errorf("expected ports [3000], got %v", result.Ports)
	}
}

func TestScanCurrentDirNoDevScript(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"name": "test-lib", "scripts": {"build": "tsc"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	result, err := ScanCurrentDir(dir, dir)
	if err != nil {
		t.Fatalf("ScanCurrentDir: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for project without dev script")
	}
}

func TestScanCurrentDirNoPackageJSON(t *testing.T) {
	dir := t.TempDir()

	_, err := ScanCurrentDir(dir, dir)
	if err == nil {
		t.Error("expected error for missing package.json")
	}
}
