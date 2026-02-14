package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry represents a single audit log entry.
type Entry struct {
	Timestamp   time.Time `json:"timestamp"`
	Action      string    `json:"action"`      // set, get, delete, rotate, list, export, user_add, user_remove, role_change
	User        string    `json:"user"`         // email or "local"
	Environment string    `json:"environment"`
	Key         string    `json:"key,omitempty"`
	Details     string    `json:"details,omitempty"`
	IPAddress   string    `json:"ip,omitempty"`
}

// Logger writes audit entries to an append-only log file.
type Logger struct {
	path string
	mu   sync.Mutex
}

// NewLogger creates a new audit logger for the given directory.
func NewLogger(vaultDir string) *Logger {
	return &Logger{
		path: filepath.Join(vaultDir, "audit.log"),
	}
}

// Log writes an audit entry to the log file.
func (l *Logger) Log(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling audit entry: %w", err)
	}

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing audit entry: %w", err)
	}

	return nil
}

// Read reads all audit entries from the log file.
func (l *Logger) Read() ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading audit log: %w", err)
	}

	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // Skip malformed entries
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// ReadFiltered reads audit entries matching the given criteria.
func (l *Logger) ReadFiltered(user, action, env string, since time.Time) ([]Entry, error) {
	all, err := l.Read()
	if err != nil {
		return nil, err
	}

	var filtered []Entry
	for _, e := range all {
		if user != "" && e.User != user {
			continue
		}
		if action != "" && e.Action != action {
			continue
		}
		if env != "" && e.Environment != env {
			continue
		}
		if !since.IsZero() && e.Timestamp.Before(since) {
			continue
		}
		filtered = append(filtered, e)
	}

	return filtered, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
