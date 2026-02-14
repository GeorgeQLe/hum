package config

import (
	"os"
	"path/filepath"
	"strings"
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

	_, err := Load(tmpDir)
	if err == nil {
		t.Fatal("Load() expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in error, got: %v", err)
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

func TestSaveLoadNewFields(t *testing.T) {
	tmpDir := t.TempDir()

	pinned := true
	apps := []App{
		{
			Name:    "api",
			Dir:     "packages/api",
			Command: "npm run dev",
			Ports:   []int{8080},
			Env:     map[string]string{"NODE_ENV": "development", "DEBUG": "true"},
			DependsOn: []string{"db"},
			Group:   "backend",
			HealthCheck: &HealthCheckConfig{
				URL:      "http://localhost:8080/health",
				Interval: 5000,
			},
			Pinned:   &pinned,
			Commands: map[string]string{"dev": "npm run dev", "build": "npm run build"},
		},
		{
			Name:    "db",
			Dir:     "packages/db",
			Command: "docker compose up",
			Ports:   []int{5432},
			Group:   "backend",
		},
	}

	if err := Save(tmpDir, apps); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(loaded) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(loaded))
	}

	api := loaded[0]
	if api.Env["NODE_ENV"] != "development" {
		t.Errorf("expected Env[NODE_ENV]=development, got %q", api.Env["NODE_ENV"])
	}
	if api.Env["DEBUG"] != "true" {
		t.Errorf("expected Env[DEBUG]=true, got %q", api.Env["DEBUG"])
	}
	if len(api.DependsOn) != 1 || api.DependsOn[0] != "db" {
		t.Errorf("expected DependsOn=[db], got %v", api.DependsOn)
	}
	if api.Group != "backend" {
		t.Errorf("expected Group=backend, got %q", api.Group)
	}
	if api.HealthCheck == nil {
		t.Fatal("expected non-nil HealthCheck")
	}
	if api.HealthCheck.URL != "http://localhost:8080/health" {
		t.Errorf("expected HealthCheck.URL, got %q", api.HealthCheck.URL)
	}
	if api.HealthCheck.Interval != 5000 {
		t.Errorf("expected HealthCheck.Interval=5000, got %d", api.HealthCheck.Interval)
	}
	if api.Pinned == nil || !*api.Pinned {
		t.Error("expected Pinned=true")
	}
	if api.Commands["dev"] != "npm run dev" {
		t.Errorf("expected Commands[dev], got %q", api.Commands["dev"])
	}
	if api.Commands["build"] != "npm run build" {
		t.Errorf("expected Commands[build], got %q", api.Commands["build"])
	}

	// Second app should have no optional fields
	db := loaded[1]
	if len(db.Env) != 0 {
		t.Errorf("expected empty Env for db, got %v", db.Env)
	}
	if db.HealthCheck != nil {
		t.Error("expected nil HealthCheck for db")
	}
}

func TestHasChangedNewFields(t *testing.T) {
	base := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}}

	// Env change
	withEnv := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Env: map[string]string{"X": "1"}}
	if !HasChanged(base, withEnv) {
		t.Error("HasChanged() should detect env difference")
	}

	// Group change
	withGroup := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Group: "backend"}
	if !HasChanged(base, withGroup) {
		t.Error("HasChanged() should detect group difference")
	}

	// DependsOn change
	withDeps := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, DependsOn: []string{"db"}}
	if !HasChanged(base, withDeps) {
		t.Error("HasChanged() should detect dependsOn difference")
	}

	// Commands change
	withCmds := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Commands: map[string]string{"dev": "npm dev"}}
	if !HasChanged(base, withCmds) {
		t.Error("HasChanged() should detect commands difference")
	}

	// HealthCheck change
	withHC := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, HealthCheck: &HealthCheckConfig{URL: "http://localhost:3000/health", Interval: 5000}}
	if !HasChanged(base, withHC) {
		t.Error("HasChanged() should detect healthCheck difference")
	}
}

func TestValidateDependencies(t *testing.T) {
	// Valid deps
	apps := []App{
		{Name: "api", Dir: ".", Command: "npm dev", Ports: []int{8080}, DependsOn: []string{"db"}},
		{Name: "db", Dir: ".", Command: "docker up", Ports: []int{5432}},
	}
	if err := ValidateDependencies(apps); err != nil {
		t.Errorf("expected no error for valid deps, got: %v", err)
	}

	// Missing reference
	badApps := []App{
		{Name: "api", Dir: ".", Command: "npm dev", Ports: []int{8080}, DependsOn: []string{"missing"}},
	}
	if err := ValidateDependencies(badApps); err == nil {
		t.Error("expected error for missing dependency reference")
	}

	// Self-reference
	selfRef := []App{
		{Name: "api", Dir: ".", Command: "npm dev", Ports: []int{8080}, DependsOn: []string{"api"}},
	}
	if err := ValidateDependencies(selfRef); err == nil {
		t.Error("expected error for self-referencing dependency")
	}
}

func TestValidateNewFields(t *testing.T) {
	// Empty DependsOn entry
	app := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, DependsOn: []string{""}}
	if err := app.Validate(); err == nil {
		t.Error("expected error for empty dependsOn entry")
	}

	// HealthCheck with empty URL
	app2 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, HealthCheck: &HealthCheckConfig{URL: "", Interval: 5000}}
	if err := app2.Validate(); err == nil {
		t.Error("expected error for empty healthCheck URL")
	}

	// Commands with empty value
	app3 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Commands: map[string]string{"dev": ""}}
	if err := app3.Validate(); err == nil {
		t.Error("expected error for empty command value")
	}

	// Valid HealthCheck
	app4 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, HealthCheck: &HealthCheckConfig{URL: "http://localhost:3000/health", Interval: 5000}}
	if err := app4.Validate(); err != nil {
		t.Errorf("expected no error for valid healthCheck, got: %v", err)
	}

	// Watch extension without dot
	app5 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{Extensions: []string{"go"}}}
	if err := app5.Validate(); err == nil {
		t.Error("expected error for watch extension without dot")
	}

	// Valid watch config
	app6 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{Extensions: []string{".go", ".ts"}}}
	if err := app6.Validate(); err != nil {
		t.Errorf("expected no error for valid watch config, got: %v", err)
	}

	// Empty watch config (valid — enables watching with defaults)
	app7 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{}}
	if err := app7.Validate(); err != nil {
		t.Errorf("expected no error for empty watch config, got: %v", err)
	}
}

func TestSaveLoadWatchConfig(t *testing.T) {
	tmpDir := t.TempDir()

	apps := []App{
		{
			Name:    "api",
			Dir:     "packages/api",
			Command: "go run .",
			Ports:   []int{8080},
			Watch: &WatchConfig{
				Paths:      []string{"./src"},
				Extensions: []string{".go"},
				Ignore:     []string{"vendor"},
			},
		},
	}

	if err := Save(tmpDir, apps); err != nil {
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
	if app.Watch == nil {
		t.Fatal("expected non-nil Watch")
	}
	if len(app.Watch.Paths) != 1 || app.Watch.Paths[0] != "./src" {
		t.Errorf("expected Paths=[./src], got %v", app.Watch.Paths)
	}
	if len(app.Watch.Extensions) != 1 || app.Watch.Extensions[0] != ".go" {
		t.Errorf("expected Extensions=[.go], got %v", app.Watch.Extensions)
	}
	if len(app.Watch.Ignore) != 1 || app.Watch.Ignore[0] != "vendor" {
		t.Errorf("expected Ignore=[vendor], got %v", app.Watch.Ignore)
	}
}

func TestHasChangedWatchConfig(t *testing.T) {
	base := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}}
	withWatch := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{}}
	if !HasChanged(base, withWatch) {
		t.Error("HasChanged() should detect watch config added")
	}

	w1 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{Extensions: []string{".go"}}}
	w2 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{Extensions: []string{".ts"}}}
	if !HasChanged(w1, w2) {
		t.Error("HasChanged() should detect watch extension change")
	}

	// Identical watch configs
	w3 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{Extensions: []string{".go"}}}
	w4 := App{Name: "web", Dir: ".", Command: "npm dev", Ports: []int{3000}, Watch: &WatchConfig{Extensions: []string{".go"}}}
	if HasChanged(w3, w4) {
		t.Error("HasChanged() should not detect change for identical watch configs")
	}
}
