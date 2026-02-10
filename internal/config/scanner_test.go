package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsServerDevScript(t *testing.T) {
	tests := []struct {
		script string
		server bool
	}{
		{"next dev", true},
		{"vite", true},
		{"wrangler dev", true},
		{"expo start", true},
		{"nodemon server.js", true},
		{"tsx watch src/index.ts", true},
		{"PORT=3000 node server.js", true},
		{"node server.js --port 3000", true},
		{"node server.js -p 8080", true},
		{"tsc --watch", false},
		{"tsup --watch", false},
		{"echo hello", false},
		{"jest --watch", false},
	}

	for _, tt := range tests {
		got := IsServerDevScript(tt.script)
		if got != tt.server {
			t.Errorf("IsServerDevScript(%q) = %v, want %v", tt.script, got, tt.server)
		}
	}
}

func TestDetectPorts(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		script string
		ports  []int
	}{
		{"PORT=4000 node server.js", []int{4000}},
		{"node server.js --port 8080", []int{8080}},
		{"node server.js -p 9090", []int{9090}},
		{"next dev", []int{3000}},
		{"vite", []int{5173}},
		{"wrangler dev", []int{8787}},
		{"expo start", []int{8081}},
	}

	for _, tt := range tests {
		got := DetectPorts(tt.script, dir)
		if len(got) != len(tt.ports) {
			t.Errorf("DetectPorts(%q) = %v, want %v", tt.script, got, tt.ports)
			continue
		}
		for i := range got {
			if got[i] != tt.ports[i] {
				t.Errorf("DetectPorts(%q) = %v, want %v", tt.script, got, tt.ports)
			}
		}
	}
}

func TestDetectPortsViteConfig(t *testing.T) {
	dir := t.TempDir()
	viteConfig := `export default { server: { port: 4000 } }`
	os.WriteFile(filepath.Join(dir, "vite.config.ts"), []byte(viteConfig), 0644)

	ports := DetectPorts("vite", dir)
	if len(ports) != 1 || ports[0] != 4000 {
		t.Errorf("expected [4000] from vite.config.ts, got %v", ports)
	}
}

func TestDetectPackageManager(t *testing.T) {
	root := t.TempDir()

	// pnpm
	pnpmDir := filepath.Join(root, "pnpm-project")
	os.MkdirAll(pnpmDir, 0755)
	os.WriteFile(filepath.Join(pnpmDir, "pnpm-lock.yaml"), []byte(""), 0644)
	if pm := DetectPackageManager(pnpmDir, "", root); pm != "pnpm" {
		t.Errorf("expected pnpm, got %s", pm)
	}

	// yarn
	yarnDir := filepath.Join(root, "yarn-project")
	os.MkdirAll(yarnDir, 0755)
	os.WriteFile(filepath.Join(yarnDir, "yarn.lock"), []byte(""), 0644)
	if pm := DetectPackageManager(yarnDir, "", root); pm != "yarn" {
		t.Errorf("expected yarn, got %s", pm)
	}

	// packageManager field
	if pm := DetectPackageManager(root, "pnpm@8.0.0", root); pm != "pnpm" {
		t.Errorf("expected pnpm from field, got %s", pm)
	}

	// fallback
	emptyDir := filepath.Join(root, "empty-project")
	os.MkdirAll(emptyDir, 0755)
	if pm := DetectPackageManager(emptyDir, "", root); pm != "npm" {
		t.Errorf("expected npm fallback, got %s", pm)
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		pkgName string
		relDir  string
		want    string
	}{
		{"my-app", "packages/my-app", "my-app"},
		{"@scope/my-lib", "packages/my-lib", "my-lib"},
		{"", "packages/cool-app", "cool-app"},
		{"", ".", "."},
	}

	for _, tt := range tests {
		got := ExtractName(tt.pkgName, tt.relDir)
		if got != tt.want {
			t.Errorf("ExtractName(%q, %q) = %q, want %q", tt.pkgName, tt.relDir, got, tt.want)
		}
	}
}

func TestDetectApps(t *testing.T) {
	root := t.TempDir()

	// Create a project with a dev script
	appDir := filepath.Join(root, "my-app")
	os.MkdirAll(appDir, 0755)
	pkg := `{"name": "my-app", "scripts": {"dev": "next dev"}}`
	os.WriteFile(filepath.Join(appDir, "package.json"), []byte(pkg), 0644)

	candidates := DetectApps(root, nil)
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Name != "my-app" {
		t.Errorf("expected name 'my-app', got %q", candidates[0].Name)
	}
	if len(candidates[0].Ports) == 0 || candidates[0].Ports[0] != 3000 {
		t.Errorf("expected ports [3000], got %v", candidates[0].Ports)
	}
}

func TestDetectAppsExcludesExisting(t *testing.T) {
	root := t.TempDir()

	appDir := filepath.Join(root, "my-app")
	os.MkdirAll(appDir, 0755)
	pkg := `{"name": "my-app", "scripts": {"dev": "next dev"}}`
	os.WriteFile(filepath.Join(appDir, "package.json"), []byte(pkg), 0644)

	existing := []App{{Name: "my-app", Dir: "my-app", Command: "next dev", Ports: []int{3000}}}
	candidates := DetectApps(root, existing)
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates (existing should be excluded), got %d", len(candidates))
	}
}
