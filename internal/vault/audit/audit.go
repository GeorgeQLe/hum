package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	HMAC        string    `json:"hmac,omitempty"` // HMAC-SHA256 chain integrity hash
}

// Logger writes audit entries to an append-only log file.
type Logger struct {
	path    string
	hmacKey []byte // HMAC key for chain integrity; nil disables chaining
	mu      sync.Mutex
}

// NewLogger creates a new audit logger for the given directory.
func NewLogger(vaultDir string) *Logger {
	return &Logger{
		path: filepath.Join(vaultDir, "audit.log"),
	}
}

// NewLoggerWithHMAC creates an audit logger with HMAC chain integrity.
// The key should be derived from the vault encryption key.
func NewLoggerWithHMAC(vaultDir string, hmacKey []byte) *Logger {
	return &Logger{
		path:    filepath.Join(vaultDir, "audit.log"),
		hmacKey: hmacKey,
	}
}

// computeHMAC computes HMAC-SHA256(prevHMAC + entryJSON) for chain integrity.
func computeHMAC(key []byte, prevHMAC string, entryJSON []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(prevHMAC))
	mac.Write(entryJSON)
	return hex.EncodeToString(mac.Sum(nil))
}

// lastHMAC reads the HMAC from the last entry in the log file.
// Returns empty string if no entries exist.
func (l *Logger) lastHMAC() string {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return ""
	}
	lines := splitLines(data)
	for i := len(lines) - 1; i >= 0; i-- {
		if len(lines[i]) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(lines[i], &entry); err == nil {
			return entry.HMAC
		}
	}
	return ""
}

// Log writes an audit entry to the log file.
func (l *Logger) Log(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	// Compute HMAC chain if key is configured
	if len(l.hmacKey) > 0 {
		prevHMAC := l.lastHMAC()
		// Marshal without HMAC first to get the entry JSON for signing
		entry.HMAC = ""
		entryJSON, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshaling audit entry for HMAC: %w", err)
		}
		entry.HMAC = computeHMAC(l.hmacKey, prevHMAC, entryJSON)
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

// VerifyChain verifies the HMAC chain integrity of the entire audit log.
// Returns nil if the chain is valid, or an error describing the first broken link.
func (l *Logger) VerifyChain(hmacKey []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading audit log: %w", err)
	}

	prevHMAC := ""
	lineNum := 0
	for _, line := range splitLines(data) {
		lineNum++
		if len(line) == 0 {
			continue
		}

		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("line %d: parse error: %w", lineNum, err)
		}

		if entry.HMAC == "" {
			// Entry predates HMAC chain — skip but reset chain
			prevHMAC = ""
			continue
		}

		recordedHMAC := entry.HMAC
		// Recompute: marshal entry without HMAC to get original payload
		entry.HMAC = ""
		entryJSON, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("line %d: remarshal error: %w", lineNum, err)
		}

		expectedHMAC := computeHMAC(hmacKey, prevHMAC, entryJSON)
		if recordedHMAC != expectedHMAC {
			return fmt.Errorf("line %d: HMAC chain broken (tampered or reordered entry)", lineNum)
		}

		prevHMAC = recordedHMAC
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
	var parseErrors []error
	lineNum := 0
	for _, line := range splitLines(data) {
		lineNum++
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			parseErrors = append(parseErrors, fmt.Errorf("line %d: %w", lineNum, err))
			continue
		}
		entries = append(entries, entry)
	}

	if len(parseErrors) > 0 {
		return entries, fmt.Errorf("encountered %d parse errors (first: %v)", len(parseErrors), parseErrors[0])
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
