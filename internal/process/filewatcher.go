package process

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/panicutil"
)

const (
	fileWatchDebounce = 300 * time.Millisecond
	eventBufferSize   = 64
)

// Default directories to ignore when watching.
var defaultIgnoreDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	".next": true, "__pycache__": true, "venv": true, ".turbo": true,
	".cache": true, "coverage": true, "tmp": true, ".idea": true,
	".vscode": true,
}

// Default file patterns to ignore.
var defaultIgnorePatterns = []string{
	"*.swp", "*.swo", "*~", ".DS_Store", "*.lock",
}

// FileWatchEvent is emitted when a watched file changes.
type FileWatchEvent struct {
	AppName  string
	FilePath string
	Time     time.Time
}

type appWatcher struct {
	watcher        *fsnotify.Watcher
	baseDir        string
	extensions     map[string]bool // empty means all extensions
	ignoreDirs     map[string]bool
	ignorePatterns []string
	enabled        bool
	restartPending bool
	stopCh         chan struct{}
	mu             sync.Mutex
}

// FileWatchManager manages per-app file watchers.
type FileWatchManager struct {
	mu      sync.Mutex
	apps    map[string]*appWatcher
	eventCh chan FileWatchEvent
}

// NewFileWatchManager creates a new FileWatchManager.
func NewFileWatchManager() *FileWatchManager {
	return &FileWatchManager{
		apps:    make(map[string]*appWatcher),
		eventCh: make(chan FileWatchEvent, eventBufferSize),
	}
}

// Events returns the channel that emits file watch events.
func (fm *FileWatchManager) Events() <-chan FileWatchEvent {
	return fm.eventCh
}

// Register starts watching files for the named app.
func (fm *FileWatchManager) Register(appName, baseDir string, cfg *config.WatchConfig) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Stop existing watcher if any
	if existing, ok := fm.apps[appName]; ok {
		close(existing.stopCh)
		existing.watcher.Close()
		delete(fm.apps, appName)
	}

	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	aw := &appWatcher{
		watcher:        fw,
		baseDir:        baseDir,
		extensions:     make(map[string]bool),
		ignoreDirs:     make(map[string]bool),
		ignorePatterns: append([]string{}, defaultIgnorePatterns...),
		enabled:        true,
		stopCh:         make(chan struct{}),
	}

	// Copy default ignore dirs
	for k, v := range defaultIgnoreDirs {
		aw.ignoreDirs[k] = v
	}

	// Apply config
	if cfg != nil {
		for _, ext := range cfg.Extensions {
			aw.extensions[ext] = true
		}
		for _, ign := range cfg.Ignore {
			// Could be a dir name or a glob pattern
			if strings.ContainsAny(ign, "*?[") {
				aw.ignorePatterns = append(aw.ignorePatterns, ign)
			} else {
				aw.ignoreDirs[ign] = true
			}
		}
	}

	// Determine paths to watch
	paths := []string{"."}
	if cfg != nil && len(cfg.Paths) > 0 {
		paths = cfg.Paths
	}

	// Walk and add directories
	for _, p := range paths {
		watchPath := filepath.Join(baseDir, p)
		if err := aw.addDirRecursive(watchPath); err != nil {
			fw.Close()
			return err
		}
	}

	fm.apps[appName] = aw
	go func() {
		defer panicutil.Recover("file watcher")
		fm.watchLoop(appName, aw)
	}()

	return nil
}

// Unregister stops watching for the named app.
func (fm *FileWatchManager) Unregister(appName string) {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if aw, ok := fm.apps[appName]; ok {
		close(aw.stopCh)
		aw.watcher.Close()
		delete(fm.apps, appName)
	}
}

// Toggle enables or disables watching for the named app. Returns new state.
func (fm *FileWatchManager) Toggle(appName string) bool {
	fm.mu.Lock()
	aw, ok := fm.apps[appName]
	fm.mu.Unlock()
	if !ok {
		return false
	}
	aw.mu.Lock()
	defer aw.mu.Unlock()
	aw.enabled = !aw.enabled
	return aw.enabled
}

// SetEnabled explicitly sets the enabled state for an app watcher.
func (fm *FileWatchManager) SetEnabled(appName string, enabled bool) {
	fm.mu.Lock()
	aw, ok := fm.apps[appName]
	fm.mu.Unlock()
	if !ok {
		return
	}
	aw.mu.Lock()
	defer aw.mu.Unlock()
	aw.enabled = enabled
}

// SetRestartInFlight suppresses events while a restart is in progress.
func (fm *FileWatchManager) SetRestartInFlight(appName string, inFlight bool) {
	fm.mu.Lock()
	aw, ok := fm.apps[appName]
	fm.mu.Unlock()
	if !ok {
		return
	}
	aw.mu.Lock()
	defer aw.mu.Unlock()
	aw.restartPending = inFlight
}

// IsEnabled returns whether watching is enabled for the app.
func (fm *FileWatchManager) IsEnabled(appName string) bool {
	fm.mu.Lock()
	aw, ok := fm.apps[appName]
	fm.mu.Unlock()
	if !ok {
		return false
	}
	aw.mu.Lock()
	defer aw.mu.Unlock()
	return aw.enabled
}

// HasWatch returns whether the app has a registered watcher.
func (fm *FileWatchManager) HasWatch(appName string) bool {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	_, ok := fm.apps[appName]
	return ok
}

// StopAll stops all watchers.
func (fm *FileWatchManager) StopAll() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	for name, aw := range fm.apps {
		close(aw.stopCh)
		aw.watcher.Close()
		delete(fm.apps, name)
	}
}

func (fm *FileWatchManager) watchLoop(appName string, aw *appWatcher) {
	var debounceTimer *time.Timer
	var lastFile string

	for {
		select {
		case <-aw.stopCh:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-aw.watcher.Events:
			if !ok {
				return
			}

			// Handle new directories
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					aw.addDirRecursive(event.Name) //nolint:errcheck // best-effort watch
					continue
				}
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}

			aw.mu.Lock()
			enabled := aw.enabled
			restarting := aw.restartPending
			aw.mu.Unlock()

			if !enabled || restarting {
				continue
			}

			if !aw.shouldWatch(event.Name) {
				continue
			}

			lastFile = event.Name

			// Debounce
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			f := lastFile
			debounceTimer = time.AfterFunc(fileWatchDebounce, func() {
				select {
				case fm.eventCh <- FileWatchEvent{
					AppName:  appName,
					FilePath: f,
					Time:     time.Now(),
				}:
				default:
				}
			})

		case _, ok := <-aw.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (aw *appWatcher) shouldWatch(filePath string) bool {
	name := filepath.Base(filePath)

	// Check ignore patterns
	for _, pattern := range aw.ignorePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return false
		}
	}

	// Check extensions
	if len(aw.extensions) > 0 {
		ext := filepath.Ext(filePath)
		if !aw.extensions[ext] {
			return false
		}
	}

	return true
}

func (aw *appWatcher) addDirRecursive(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		if !info.IsDir() {
			return nil
		}
		name := info.Name()
		if aw.ignoreDirs[name] && path != root {
			return filepath.SkipDir
		}
		return aw.watcher.Add(path)
	})
}
