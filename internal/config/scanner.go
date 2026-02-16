package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Pre-compiled regexes for IsServerDevScript and DetectPorts (C1)
var (
	excludeTscWatch  = regexp.MustCompile(`\btsc\b.*--watch`)
	excludeTsupWatch = regexp.MustCompile(`\btsup\b.*--watch`)

	serverNext     = regexp.MustCompile(`\bnext\b`)
	serverVite     = regexp.MustCompile(`\bvite\b`)
	serverWrangler = regexp.MustCompile(`\bwrangler\b`)
	serverExpo     = regexp.MustCompile(`\bexpo\b`)
	serverNodemon  = regexp.MustCompile(`\bnodemon\b`)
	serverTsx      = regexp.MustCompile(`\btsx\s+watch\b`)
	serverPort     = regexp.MustCompile(`\bPORT=`)
	serverPortFlag = regexp.MustCompile(`--port\b`)
	serverPFlag    = regexp.MustCompile(`-p\s+\d+`)
	serverNode     = regexp.MustCompile(`\bnode\s+\S`)
	serverTsNode   = regexp.MustCompile(`\bts-node\b`)
	serverRemix    = regexp.MustCompile(`\bremix\b`)
	serverNuxt     = regexp.MustCompile(`\bnuxt\b`)
	serverAstro    = regexp.MustCompile(`\bastro\b`)
	serverSvelte   = regexp.MustCompile(`\bsvelte-kit\b|@sveltejs/kit`)
	serverServe    = regexp.MustCompile(`\bserve\b|http-server`)
	serverHono     = regexp.MustCompile(`\bhono\b`)
	serverFastify  = regexp.MustCompile(`\bfastify\b`)

	portEnvRe     = regexp.MustCompile(`PORT=(\d+)`)
	portFlagRe    = regexp.MustCompile(`(?:-p\s+|--port\s+)(\d+)`)
	portConfigRe  = regexp.MustCompile(`port\s*:\s*(\d+)`)
	portTomlRe    = regexp.MustCompile(`port\s*=\s*(\d+)`)
)

// ScanSkipDirs are directories to skip during scanning.
var ScanSkipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".next":        true,
	"dist":         true,
	"build":        true,
	".turbo":       true,
	"_archive":     true,
	"clones":       true,
	"starters":     true,
	"archive":      true,
}

// ScanCandidate represents a detected app candidate.
type ScanCandidate struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Command   string `json:"command"`
	Ports     []int  `json:"ports"`
	DevScript string `json:"devScript"`
}

type packageJSON struct {
	Name           string            `json:"name"`
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
}

type walkResult struct {
	fullPath string
	pkg      packageJSON
}

// DetectApps scans the project root for package.json files with dev scripts.
func DetectApps(projectRoot string, existingApps []App) []ScanCandidate {
	found := walkForPackageJSONs(projectRoot, 5, 0)

	// Identify monorepo roots that have child results
	monorepoRoots := make(map[string]bool)
	for _, f := range found {
		if isMonorepoRoot(f.fullPath) {
			for _, other := range found {
				if other.fullPath != f.fullPath && strings.HasPrefix(other.fullPath, f.fullPath+string(filepath.Separator)) {
					monorepoRoots[f.fullPath] = true
					break
				}
			}
		}
	}

	registeredDirs := make(map[string]bool)
	for _, a := range existingApps {
		absDir := filepath.Join(projectRoot, a.Dir)
		registeredDirs[absDir] = true
	}

	var candidates []ScanCandidate
	for _, f := range found {
		if monorepoRoots[f.fullPath] {
			continue
		}

		// Try scripts.dev first, then fall back to scripts.start
		devScript := f.pkg.Scripts["dev"]
		scriptName := "dev"
		if !IsServerDevScript(devScript) {
			startScript := f.pkg.Scripts["start"]
			if IsServerDevScript(startScript) {
				devScript = startScript
				scriptName = "start"
			} else {
				continue
			}
		}

		if registeredDirs[f.fullPath] {
			continue
		}

		ports := DetectPorts(devScript, f.fullPath)

		relDir, err := filepath.Rel(projectRoot, f.fullPath)
		if err != nil {
			continue
		}
		name := ExtractName(f.pkg.Name, relDir)
		pm := DetectPackageManager(f.fullPath, f.pkg.PackageManager, projectRoot)
		command := buildCommandForScript(pm, scriptName)

		candidates = append(candidates, ScanCandidate{
			Name:      name,
			Dir:       relDir,
			Command:   command,
			Ports:     ports,
			DevScript: devScript,
		})
	}

	return candidates
}

// ScanCurrentDir detects a single app from the given directory.
func ScanCurrentDir(dir, projectRoot string) (*ScanCandidate, error) {
	pkgPath := filepath.Join(dir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, err
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	devScript := pkg.Scripts["dev"]
	scriptName := "dev"
	if devScript == "" {
		devScript = pkg.Scripts["start"]
		scriptName = "start"
	}
	if devScript == "" {
		return nil, nil
	}

	relDir, err := filepath.Rel(projectRoot, dir)
	if err != nil {
		relDir = "."
	}
	if relDir == "" {
		relDir = "."
	}

	name := ExtractName(pkg.Name, relDir)
	ports := DetectPorts(devScript, dir)
	pm := DetectPackageManager(dir, pkg.PackageManager, projectRoot)
	command := buildCommandForScript(pm, scriptName)

	return &ScanCandidate{
		Name:      name,
		Dir:       relDir,
		Command:   command,
		Ports:     ports,
		DevScript: devScript,
	}, nil
}

func walkForPackageJSONs(baseDir string, maxDepth, depth int) []walkResult {
	if depth > maxDepth {
		return nil
	}

	var results []walkResult

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return results
	}

	pkgPath := filepath.Join(baseDir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg packageJSON
		if json.Unmarshal(data, &pkg) == nil && (pkg.Scripts["dev"] != "" || pkg.Scripts["start"] != "") {
			results = append(results, walkResult{fullPath: baseDir, pkg: pkg})
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if ScanSkipDirs[entry.Name()] {
			continue
		}
		results = append(results, walkForPackageJSONs(filepath.Join(baseDir, entry.Name()), maxDepth, depth+1)...)
	}

	return results
}

func isMonorepoRoot(fullPath string) bool {
	if _, err := os.Stat(filepath.Join(fullPath, "turbo.json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(fullPath, "pnpm-workspace.yaml")); err == nil {
		return true
	}
	return false
}

// IsServerDevScript checks if a dev script is for a server.
func IsServerDevScript(devScript string) bool {
	if excludeTscWatch.MatchString(devScript) || excludeTsupWatch.MatchString(devScript) {
		return false
	}

	serverPatterns := []*regexp.Regexp{
		serverNext, serverVite, serverWrangler, serverExpo,
		serverNodemon, serverTsx, serverPort, serverPortFlag, serverPFlag,
		serverNode, serverTsNode, serverRemix, serverNuxt,
		serverAstro, serverSvelte, serverServe, serverHono, serverFastify,
	}
	for _, re := range serverPatterns {
		if re.MatchString(devScript) {
			return true
		}
	}
	return false
}

// DetectPorts detects port numbers from a dev script and config files.
func DetectPorts(devScript, fullPath string) []int {
	// 1. PORT=(\d+) in dev script
	if m := portEnvRe.FindStringSubmatch(devScript); len(m) > 1 {
		return parsePort(m[1])
	}

	// 2. -p or --port in dev script
	if m := portFlagRe.FindStringSubmatch(devScript); len(m) > 1 {
		return parsePort(m[1])
	}

	// 3. port in vite.config
	for _, ext := range []string{"ts", "js", "mjs"} {
		vitePath := filepath.Join(fullPath, "vite.config."+ext)
		if data, err := os.ReadFile(vitePath); err == nil {
			if m := portConfigRe.FindStringSubmatch(string(data)); len(m) > 1 {
				return parsePort(m[1])
			}
		}
	}

	// 4. port in wrangler.toml
	wranglerPath := filepath.Join(fullPath, "wrangler.toml")
	if data, err := os.ReadFile(wranglerPath); err == nil {
		if m := portTomlRe.FindStringSubmatch(string(data)); len(m) > 1 {
			return parsePort(m[1])
		}
	}

	// 5. Framework defaults
	if serverNext.MatchString(devScript) {
		return []int{3000}
	}
	if serverVite.MatchString(devScript) {
		return []int{5173}
	}
	if serverWrangler.MatchString(devScript) {
		return []int{8787}
	}
	if serverExpo.MatchString(devScript) {
		return []int{8081}
	}

	return nil
}

func parsePort(s string) []int {
	var p int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			p = p*10 + int(c-'0')
		}
	}
	if p > 0 && p < 65536 {
		return []int{p}
	}
	return nil
}

// DetectPackageManager detects the package manager for a project.
func DetectPackageManager(fullPath, packageManagerField, projectRoot string) string {
	if packageManagerField != "" {
		if strings.HasPrefix(packageManagerField, "pnpm") {
			return "pnpm"
		}
		if strings.HasPrefix(packageManagerField, "yarn") {
			return "yarn"
		}
		if strings.HasPrefix(packageManagerField, "npm") {
			return "npm"
		}
	}

	dir := fullPath
	for {
		if _, err := os.Stat(filepath.Join(dir, "pnpm-lock.yaml")); err == nil {
			return "pnpm"
		}
		if _, err := os.Stat(filepath.Join(dir, "yarn.lock")); err == nil {
			return "yarn"
		}
		if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); err == nil {
			return "npm"
		}
		parent := filepath.Dir(dir)
		if parent == dir || !strings.HasPrefix(dir, projectRoot) {
			break
		}
		dir = parent
	}

	return "npm"
}

func buildCommandForScript(pm, scriptName string) string {
	switch pm {
	case "pnpm":
		return "pnpm " + scriptName
	case "yarn":
		return "yarn " + scriptName
	default:
		if scriptName == "start" {
			return "npm start"
		}
		return "npm run " + scriptName
	}
}

// ExtractName extracts the app name from package name or directory.
func ExtractName(pkgName, relDir string) string {
	if pkgName != "" {
		// Remove @scope/ prefix
		if idx := strings.LastIndex(pkgName, "/"); idx >= 0 {
			pkgName = pkgName[idx+1:]
		}
		return pkgName
	}
	return filepath.Base(relDir)
}
