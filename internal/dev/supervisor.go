package dev

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/georgele/hum/internal/ipc"
)

const (
	buildTimeout     = 60 * time.Second
	debounceDelay    = 500 * time.Millisecond
	childGracePeriod = 5 * time.Second
)

// Directories to skip when watching Go source files.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "web": true,
	"dist": true, "build": true, "tmp": true,
}

// loopResult indicates what happened at the end of a child loop iteration.
type loopResult int

const (
	loopExit    loopResult = iota // child exited normally (user quit)
	loopRebuild                  // child exited after rebuild signal
)

// Supervisor watches Go source files and rebuilds/restarts humrun.
type Supervisor struct {
	projectDir string
	tmpBinary  string
	firstRun   bool
}

// New creates a supervisor for the given project directory.
func New(projectDir string) (*Supervisor, error) {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project dir: %w", err)
	}
	hash := sha256.Sum256([]byte(abs))
	tmpBinary := filepath.Join(os.TempDir(), fmt.Sprintf("humrun-dev-%x", hash[:8]))
	return &Supervisor{
		projectDir: abs,
		tmpBinary:  tmpBinary,
		firstRun:   true,
	}, nil
}

// Run starts the supervisor loop: build → launch child → watch → rebuild → restart.
func (s *Supervisor) Run() error {
	// Trap SIGINT/SIGTERM for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	if err := s.addGoFiles(watcher); err != nil {
		return fmt.Errorf("failed to watch files: %w", err)
	}

	// Initial build
	fmt.Println("[dev] Building...")
	if err := s.build(); err != nil {
		fmt.Fprintf(os.Stderr, "[dev] Build failed:\n%s\n", err)
		fmt.Println("[dev] Waiting for file changes...")
		// Don't exit — wait for changes and try again
		return s.waitAndRetry(watcher, sigCh)
	}

	return s.runLoop(watcher, sigCh)
}

// runLoop is the outer for-loop: launch child → run child loop → check result.
func (s *Supervisor) runLoop(watcher *fsnotify.Watcher, sigCh chan os.Signal) error {
	for {
		fmt.Println("[dev] Starting humrun...")

		child, err := s.launchChild()
		if err != nil {
			return fmt.Errorf("failed to launch child: %w", err)
		}

		result, err := s.runChildLoop(child, watcher, sigCh)
		if err != nil {
			return err
		}

		if result == loopExit {
			s.cleanup()
			return nil
		}

		s.firstRun = false
	}
}

// runChildLoop runs the inner select loop for a single child process.
// It returns loopRebuild if the child was restarted after a successful build,
// or loopExit if the child exited normally or a signal was received.
func (s *Supervisor) runChildLoop(child *exec.Cmd, watcher *fsnotify.Watcher, sigCh chan os.Signal) (loopResult, error) {
	childDone := make(chan error, 1)
	go func() {
		childDone <- child.Wait()
	}()

	var debounceTimer *time.Timer
	rebuildCh := make(chan struct{}, 1)
	pendingRebuild := false

	for {
		select {
		case <-sigCh:
			// Clean shutdown: kill child and exit
			fmt.Println("\n[dev] Shutting down...")
			s.stopChild(child)
			<-childDone
			s.cleanup()
			return loopExit, nil

		case event, ok := <-watcher.Events:
			if !ok {
				return loopExit, nil
			}
			if !s.isGoFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// Debounce
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceDelay, func() {
				select {
				case rebuildCh <- struct{}{}:
				default:
				}
			})

		case <-rebuildCh:
			fmt.Println("[dev] File changed, rebuilding...")
			if err := s.build(); err != nil {
				fmt.Fprintf(os.Stderr, "[dev] Build failed:\n%s\n", err)
				fmt.Println("[dev] Waiting for next change...")
				// Send build error to running TUI via IPC
				s.sendBuildError(err.Error())
				continue
			}
			fmt.Println("[dev] Build succeeded, restarting TUI...")
			pendingRebuild = true
			// Signal child to gracefully exit
			child.Process.Signal(syscall.SIGUSR1)

		case err := <-childDone:
			if pendingRebuild {
				if err != nil {
					fmt.Fprintf(os.Stderr, "[dev] child process exited: %v\n", err)
				}
				return loopRebuild, nil
			}
			// Child exited on its own (user quit)
			return loopExit, nil

		case _, ok := <-watcher.Errors:
			if !ok {
				return loopExit, nil
			}
		}
	}
}

func (s *Supervisor) build() error {
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", s.tmpBinary, ".")
	cmd.Dir = s.projectDir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Supervisor) launchChild() (*exec.Cmd, error) {
	args := []string{}
	if !s.firstRun {
		args = append(args, "--restore")
	}

	cmd := exec.Command(s.tmpBinary, args...)
	cmd.Dir = s.projectDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Don't set Setpgid — the child's managed apps already have their own pgroups

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (s *Supervisor) stopChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Send SIGTERM first
	cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(childGracePeriod):
		cmd.Process.Kill()
		<-done
	}
}

func (s *Supervisor) cleanup() {
	os.Remove(s.tmpBinary)
}

func (s *Supervisor) sendBuildError(errMsg string) {
	client := ipc.NewClient(s.projectDir)
	client.BuildError(errMsg) // best effort
}

func (s *Supervisor) isGoFile(path string) bool {
	return strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".mod") || strings.HasSuffix(path, ".sum")
}

func (s *Supervisor) addGoFiles(watcher *fsnotify.Watcher) error {
	return filepath.Walk(s.projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if skipDirs[name] && path != s.projectDir {
				return filepath.SkipDir
			}
			return watcher.Add(path)
		}
		return nil
	})
}

func (s *Supervisor) waitAndRetry(watcher *fsnotify.Watcher, sigCh chan os.Signal) error {
	var debounceTimer *time.Timer
	rebuildCh := make(chan struct{}, 1)

	for {
		select {
		case <-sigCh:
			fmt.Println("\n[dev] Shutting down...")
			s.cleanup()
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !s.isGoFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceDelay, func() {
				select {
				case rebuildCh <- struct{}{}:
				default:
				}
			})

		case <-rebuildCh:
			fmt.Println("[dev] Rebuilding...")
			if err := s.build(); err != nil {
				fmt.Fprintf(os.Stderr, "[dev] Build failed:\n%s\n", err)
				fmt.Println("[dev] Waiting for next change...")
				continue
			}
			// Build succeeded — enter main loop
			return s.runLoop(watcher, sigCh)

		case _, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
		}
	}
}
