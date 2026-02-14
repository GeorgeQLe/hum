package audit

import (
	"fmt"
	"io"
	"time"
)

// ExportPDF writes the compliance report as a simple text-based report.
// A full PDF implementation would use github.com/jung-kurt/gofpdf.
// This provides the report content in a print-ready format.
func (r *ComplianceReport) ExportPDF(w io.Writer) error {
	// Header
	fmt.Fprintf(w, "═══════════════════════════════════════════════════\n")
	fmt.Fprintf(w, "  ENVSAFE COMPLIANCE REPORT\n")
	fmt.Fprintf(w, "  Project: %s\n", r.Project)
	fmt.Fprintf(w, "  Generated: %s\n", r.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "═══════════════════════════════════════════════════\n\n")

	// Summary
	fmt.Fprintf(w, "SUMMARY\n")
	fmt.Fprintf(w, "───────────────────────────────────────────────────\n")
	fmt.Fprintf(w, "  Total Secrets:      %d\n", r.TotalSecrets)
	fmt.Fprintf(w, "  Environments:       %d\n", len(r.Environments))
	fmt.Fprintf(w, "  Audit Log Entries:  %d\n", len(r.AccessLog))
	fmt.Fprintf(w, "  Team Members:       %d\n\n", len(r.UserPermissions))

	// Secret Ages
	fmt.Fprintf(w, "SECRET AGES\n")
	fmt.Fprintf(w, "───────────────────────────────────────────────────\n")
	fmt.Fprintf(w, "  %-20s %-20s %s\n", "ENVIRONMENT", "KEY", "AGE (days)")
	for _, s := range r.SecretAges {
		marker := ""
		if s.AgeDays > 90 {
			marker = " [STALE]"
		}
		fmt.Fprintf(w, "  %-20s %-20s %d%s\n", s.Environment, s.Key, s.AgeDays, marker)
	}
	fmt.Fprintln(w)

	// User Permissions
	fmt.Fprintf(w, "USER PERMISSIONS\n")
	fmt.Fprintf(w, "───────────────────────────────────────────────────\n")
	fmt.Fprintf(w, "  %-30s %-12s %s\n", "EMAIL", "ROLE", "ENVIRONMENTS")
	for _, u := range r.UserPermissions {
		envStr := "all"
		if len(u.Environments) > 0 {
			envStr = fmt.Sprintf("%v", u.Environments)
		}
		fmt.Fprintf(w, "  %-30s %-12s %s\n", u.Email, u.Role, envStr)
	}
	fmt.Fprintln(w)

	// Recent Audit Log (last 50 entries)
	fmt.Fprintf(w, "RECENT ACTIVITY (last 50 entries)\n")
	fmt.Fprintf(w, "───────────────────────────────────────────────────\n")
	start := 0
	if len(r.AccessLog) > 50 {
		start = len(r.AccessLog) - 50
	}
	for _, e := range r.AccessLog[start:] {
		fmt.Fprintf(w, "  %s  %-12s %-20s %s/%s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Action, e.User, e.Environment, e.Key)
	}

	fmt.Fprintf(w, "\n═══════════════════════════════════════════════════\n")
	fmt.Fprintf(w, "  END OF REPORT\n")
	fmt.Fprintf(w, "═══════════════════════════════════════════════════\n")

	return nil
}
