package config

import (
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches apps.json for changes and notifies via a channel.
type Watcher struct {
	watcher    *fsnotify.Watcher
	changeCh   chan struct{}
	ignoreNext bool
	mu         sync.Mutex
	stopCh     chan struct{}
	stopOnce   sync.Once
}

// NewWatcher creates a new config file watcher.
func NewWatcher(configPath string) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fw.Add(configPath); err != nil {
		fw.Close()
		return nil, err
	}

	w := &Watcher{
		watcher:  fw,
		changeCh: make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
	}

	return w, nil
}

// Changes returns the channel that receives change notifications.
func (w *Watcher) Changes() <-chan struct{} {
	return w.changeCh
}

// SetIgnoreNext sets a flag to ignore the next change event.
func (w *Watcher) SetIgnoreNext() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ignoreNext = true
}

// Start begins watching for changes in a background goroutine.
func (w *Watcher) Start() {
	go func() {
		var debounceTimer *time.Timer

		for {
			select {
			case <-w.stopCh:
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				return

			case event, ok := <-w.watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}

				w.mu.Lock()
				shouldIgnore := w.ignoreNext
				if shouldIgnore {
					w.ignoreNext = false
				}
				w.mu.Unlock()

				if shouldIgnore {
					continue
				}

				// Debounce: 100ms
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
					select {
					case w.changeCh <- struct{}{}:
					default:
					}
				})

			case _, ok := <-w.watcher.Errors:
				if !ok {
					return
				}
				// Silently ignore watch errors
			}
		}
	}()
}

// Stop stops the watcher. Safe to call multiple times.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.watcher.Close()
	})
}
