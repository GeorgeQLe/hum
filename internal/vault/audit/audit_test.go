package audit

import (
	"bytes"
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
	if err := report.ExportPDF(&buf); err != nil {
		t.Fatalf("ExportPDF() error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("ExportPDF() should produce non-empty output")
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
