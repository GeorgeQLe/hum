package audit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ComplianceReport contains data for a compliance audit report.
type ComplianceReport struct {
	GeneratedAt    time.Time           `json:"generated_at"`
	Project        string              `json:"project"`
	TotalSecrets   int                 `json:"total_secrets"`
	Environments   []string            `json:"environments"`
	AccessLog      []Entry             `json:"access_log"`
	SecretAges     []SecretAge         `json:"secret_ages"`
	UserPermissions []UserPermission   `json:"user_permissions"`
}

// SecretAge tracks how old a secret is.
type SecretAge struct {
	Environment string    `json:"environment"`
	Key         string    `json:"key"`
	SetAt       time.Time `json:"set_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	AgeDays     int       `json:"age_days"`
}

// UserPermission tracks a user's role and access.
type UserPermission struct {
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Environments []string `json:"environments"`
}

// GenerateReport creates a compliance report from audit data.
func GenerateReport(project string, entries []Entry, secretAges []SecretAge, userPerms []UserPermission, envs []string) *ComplianceReport {
	return &ComplianceReport{
		GeneratedAt:     time.Now().UTC(),
		Project:         project,
		TotalSecrets:    len(secretAges),
		Environments:    envs,
		AccessLog:       entries,
		SecretAges:      secretAges,
		UserPermissions: userPerms,
	}
}

// ExportJSON writes the compliance report as JSON to the writer.
func (r *ComplianceReport) ExportJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// sanitizeCSVCell prevents CSV injection by prefixing dangerous leading
// characters with a single quote.
func sanitizeCSVCell(s string) string {
	if len(s) > 0 && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
		return "'" + s
	}
	return s
}

// ExportCSV writes the access log as CSV to the writer.
func (r *ComplianceReport) ExportCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header
	if err := cw.Write([]string{"Timestamp", "Action", "User", "Environment", "Key", "Details", "IP"}); err != nil {
		return err
	}

	for _, e := range r.AccessLog {
		if err := cw.Write([]string{
			sanitizeCSVCell(e.Timestamp.Format(time.RFC3339)),
			sanitizeCSVCell(e.Action),
			sanitizeCSVCell(e.User),
			sanitizeCSVCell(e.Environment),
			sanitizeCSVCell(e.Key),
			sanitizeCSVCell(e.Details),
			sanitizeCSVCell(e.IPAddress),
		}); err != nil {
			return err
		}
	}

	return nil
}

// FormatSummary returns a human-readable compliance summary.
func (r *ComplianceReport) FormatSummary() string {
	summary := fmt.Sprintf("Compliance Report — %s\n", r.Project)
	summary += fmt.Sprintf("Generated: %s\n\n", r.GeneratedAt.Format(time.RFC3339))
	summary += fmt.Sprintf("Total Secrets: %d\n", r.TotalSecrets)
	summary += fmt.Sprintf("Environments: %d\n", len(r.Environments))
	summary += fmt.Sprintf("Audit Log Entries: %d\n", len(r.AccessLog))
	summary += fmt.Sprintf("Users: %d\n\n", len(r.UserPermissions))

	// Secrets needing rotation (>90 days old)
	stale := 0
	for _, s := range r.SecretAges {
		if s.AgeDays > 90 {
			stale++
		}
	}
	if stale > 0 {
		summary += fmt.Sprintf("WARNING: %d secrets are older than 90 days and should be rotated.\n", stale)
	}

	return summary
}
