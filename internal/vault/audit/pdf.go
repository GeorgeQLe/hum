package audit

import (
	"bytes"
	"fmt"
	"io"
	"time"
)

// ExportPDF writes the compliance report as a simple text-based report.
// A full PDF implementation would use github.com/jung-kurt/gofpdf.
// This provides the report content in a print-ready format.
func (r *ComplianceReport) ExportPDF(w io.Writer) error {
	var buf bytes.Buffer

	// Header
	fmt.Fprintf(&buf, "═══════════════════════════════════════════════════\n")
	fmt.Fprintf(&buf, "  ENVSAFE COMPLIANCE REPORT\n")
	fmt.Fprintf(&buf, "  Project: %s\n", r.Project)
	fmt.Fprintf(&buf, "  Generated: %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&buf, "═══════════════════════════════════════════════════\n\n")

	// Summary
	fmt.Fprintf(&buf, "SUMMARY\n")
	fmt.Fprintf(&buf, "───────────────────────────────────────────────────\n")
	fmt.Fprintf(&buf, "  Total Secrets:      %d\n", r.TotalSecrets)
	fmt.Fprintf(&buf, "  Environments:       %d\n", len(r.Environments))
	fmt.Fprintf(&buf, "  Audit Log Entries:  %d\n", len(r.AccessLog))
	fmt.Fprintf(&buf, "  Team Members:       %d\n\n", len(r.UserPermissions))

	// Secret Ages
	fmt.Fprintf(&buf, "SECRET AGES\n")
	fmt.Fprintf(&buf, "───────────────────────────────────────────────────\n")
	fmt.Fprintf(&buf, "  %-20s %-20s %s\n", "ENVIRONMENT", "KEY", "AGE (days)")
	for _, s := range r.SecretAges {
		marker := ""
		if s.AgeDays > 90 {
			marker = " [STALE]"
		}
		fmt.Fprintf(&buf, "  %-20s %-20s %d%s\n", s.Environment, s.Key, s.AgeDays, marker)
	}
	fmt.Fprintln(&buf)

	// User Permissions
	fmt.Fprintf(&buf, "USER PERMISSIONS\n")
	fmt.Fprintf(&buf, "───────────────────────────────────────────────────\n")
	fmt.Fprintf(&buf, "  %-30s %-12s %s\n", "EMAIL", "ROLE", "ENVIRONMENTS")
	for _, u := range r.UserPermissions {
		envStr := "all"
		if len(u.Environments) > 0 {
			envStr = fmt.Sprintf("%v", u.Environments)
		}
		fmt.Fprintf(&buf, "  %-30s %-12s %s\n", u.Email, u.Role, envStr)
	}
	fmt.Fprintln(&buf)

	// Recent Audit Log (last 50 entries)
	fmt.Fprintf(&buf, "RECENT ACTIVITY (last 50 entries)\n")
	fmt.Fprintf(&buf, "───────────────────────────────────────────────────\n")
	start := 0
	if len(r.AccessLog) > 50 {
		start = len(r.AccessLog) - 50
	}
	for _, e := range r.AccessLog[start:] {
		fmt.Fprintf(&buf, "  %s  %-12s %-20s %s/%s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Action, e.User, e.Environment, e.Key)
	}

	fmt.Fprintf(&buf, "\n═══════════════════════════════════════════════════\n")
	fmt.Fprintf(&buf, "  END OF REPORT\n")
	fmt.Fprintf(&buf, "═══════════════════════════════════════════════════\n")

	_, err := buf.WriteTo(w)
	return err
}
