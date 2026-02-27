package audit

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestLogAndRead(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	entry := Entry{
		Action:      "set",
		User:        "test@example.com",
		Environment: "development",
		Key:         "DB_URL",
		Details:     "set secret",
	}

	if err := logger.Log(entry); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	entries, err := logger.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Read() returned %d entries, want 1", len(entries))
	}

	if entries[0].Action != "set" {
		t.Errorf("action = %q, want %q", entries[0].Action, "set")
	}
	if entries[0].User != "test@example.com" {
		t.Errorf("user = %q, want %q", entries[0].User, "test@example.com")
	}
}

func TestLogMultiple(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	for i := 0; i < 5; i++ {
		logger.Log(Entry{Action: "get", User: "user", Environment: "dev", Key: "KEY"})
	}

	entries, err := logger.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	if len(entries) != 5 {
		t.Errorf("Read() returned %d entries, want 5", len(entries))
	}
}

func TestReadEmpty(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	entries, err := logger.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	if entries != nil {
		t.Errorf("Read() on empty log should return nil, got %v", entries)
	}
}

func TestReadFiltered(t *testing.T) {
	dir := t.TempDir()
	logger := NewLogger(dir)

	logger.Log(Entry{Action: "set", User: "alice", Environment: "dev"})
	logger.Log(Entry{Action: "get", User: "bob", Environment: "dev"})
	logger.Log(Entry{Action: "set", User: "alice", Environment: "prod"})

	// Filter by user
	entries, _ := logger.ReadFiltered("alice", "", "", time.Time{})
	if len(entries) != 2 {
		t.Errorf("filter by user: got %d entries, want 2", len(entries))
	}

	// Filter by action
	entries, _ = logger.ReadFiltered("", "get", "", time.Time{})
	if len(entries) != 1 {
		t.Errorf("filter by action: got %d entries, want 1", len(entries))
	}

	// Filter by environment
	entries, _ = logger.ReadFiltered("", "", "prod", time.Time{})
	if len(entries) != 1 {
		t.Errorf("filter by env: got %d entries, want 1", len(entries))
	}
}

func TestAuditHMACChain(t *testing.T) {
	dir := t.TempDir()
	hmacKey := []byte("test-hmac-key-for-audit-chain!!!")
	logger := NewLoggerWithHMAC(dir, hmacKey)

	// Log several entries
	for i := 0; i < 5; i++ {
		logger.Log(Entry{Action: "set", User: "admin", Environment: "dev", Key: "KEY"})
	}

	// Verify the chain is intact
	if err := logger.VerifyChain(hmacKey); err != nil {
		t.Fatalf("VerifyChain() should pass on untampered log: %v", err)
	}
}

func TestAuditTamperDetection(t *testing.T) {
	dir := t.TempDir()
	hmacKey := []byte("test-hmac-key-for-audit-chain!!!")
	logger := NewLoggerWithHMAC(dir, hmacKey)

	// Log entries
	logger.Log(Entry{Action: "set", User: "admin", Environment: "dev", Key: "KEY1"})
	logger.Log(Entry{Action: "set", User: "admin", Environment: "dev", Key: "KEY2"})
	logger.Log(Entry{Action: "delete", User: "admin", Environment: "dev", Key: "KEY1"})

	// Tamper with the log — modify the second entry
	logPath := dir + "/audit.log"
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	// Replace "KEY2" with "KEY9" in the raw file
	tampered := bytes.Replace(data, []byte(`"KEY2"`), []byte(`"KEY9"`), 1)
	if err := os.WriteFile(logPath, tampered, 0600); err != nil {
		t.Fatalf("writing tampered log: %v", err)
	}

	// Verify should now fail
	if err := logger.VerifyChain(hmacKey); err == nil {
		t.Fatal("VerifyChain() should detect tampering")
	}
}

func TestAuditHMACChainWrongKey(t *testing.T) {
	dir := t.TempDir()
	hmacKey := []byte("correct-key-for-signing-entries!")
	logger := NewLoggerWithHMAC(dir, hmacKey)

	logger.Log(Entry{Action: "set", User: "admin", Environment: "dev", Key: "KEY"})

	// Verify with wrong key should fail
	wrongKey := []byte("wrong-key-should-fail-to-verify!")
	if err := logger.VerifyChain(wrongKey); err == nil {
		t.Fatal("VerifyChain() should fail with wrong key")
	}
}

func TestAuditWithoutHMAC(t *testing.T) {
	// Logger without HMAC key should still work (backward compatible)
	dir := t.TempDir()
	logger := NewLogger(dir) // no HMAC key

	logger.Log(Entry{Action: "set", User: "admin", Environment: "dev"})

	entries, err := logger.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].HMAC != "" {
		t.Error("entries from logger without HMAC key should have no HMAC field")
	}
}

func TestComplianceReportJSON(t *testing.T) {
	report := GenerateReport("test-project",
		[]Entry{{Action: "set", User: "admin"}},
		[]SecretAge{{Environment: "dev", Key: "KEY", AgeDays: 30}},
		[]UserPermission{{Email: "admin@test.com", Role: "admin"}},
		[]string{"development"},
	)

	var buf bytes.Buffer
	if err := report.ExportJSON(&buf); err != nil {
		t.Fatalf("ExportJSON() error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("ExportJSON() should produce non-empty output")
	}
}

func TestComplianceReportCSV(t *testing.T) {
	report := GenerateReport("test-project",
		[]Entry{{Action: "set", User: "admin", Timestamp: time.Now()}},
		nil, nil, nil,
	)

	var buf bytes.Buffer
	if err := report.ExportCSV(&buf); err != nil {
		t.Fatalf("ExportCSV() error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("ExportCSV() should produce non-empty output")
	}
}

func TestComplianceReportPDF(t *testing.T) {
	report := GenerateReport("test-project",
		[]Entry{{Action: "set", User: "admin", Timestamp: time.Now(), Environment: "dev", Key: "KEY"}},
		[]SecretAge{{Environment: "dev", Key: "KEY", AgeDays: 100}},
		[]UserPermission{{Email: "admin@test.com", Role: "admin"}},
		[]string{"development"},
	)

	var buf bytes.Buffer
	if err := report.ExportText(&buf); err != nil {
		t.Fatalf("ExportText() error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("ExportText() should produce non-empty output")
	}
}

func TestFormatSummary(t *testing.T) {
	report := GenerateReport("test",
		nil,
		[]SecretAge{{AgeDays: 100}, {AgeDays: 10}},
		nil,
		[]string{"dev"},
	)

	summary := report.FormatSummary()
	if summary == "" {
		t.Error("FormatSummary() should produce non-empty output")
	}
}
