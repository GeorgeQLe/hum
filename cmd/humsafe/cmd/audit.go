package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/georgele/hum/internal/vault"
	"github.com/georgele/hum/internal/vault/audit"
	"github.com/spf13/cobra"
)

func AuditCmd() *cobra.Command {
	var user string
	var action string
	var env string
	var since string
	var format string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View audit log",
		Long: `View the audit log showing who accessed or modified secrets and when.
Supports filtering by user, action, environment, and date.`,
		Example: `  humsafe audit
  humsafe audit --action set --since 2024-01-01
  humsafe audit --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'humsafe init' first")
			}

			logger := audit.NewLogger(vault.VaultPath(projectRoot))

			var sinceTime time.Time
			if since != "" {
				sinceTime, err = time.Parse("2006-01-02", since)
				if err != nil {
					return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
				}
			}

			entries, err := logger.ReadFiltered(user, action, env, sinceTime)
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println("No audit log entries found.")
				return nil
			}

			switch format {
			case "json":
				report := audit.GenerateReport("", entries, nil, nil, nil)
				return report.ExportJSON(os.Stdout)
			case "csv":
				report := audit.GenerateReport("", entries, nil, nil, nil)
				return report.ExportCSV(os.Stdout)
			default:
				for _, e := range entries {
					ts := e.Timestamp.Format("2006-01-02 15:04:05")
					fmt.Printf("[%s] %-8s %-20s %s/%s", ts, e.Action, e.User, e.Environment, e.Key)
					if e.Details != "" {
						fmt.Printf(" — %s", e.Details)
					}
					fmt.Println()
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&user, "user", "", "Filter by user email")
	cmd.Flags().StringVar(&action, "action", "", "Filter by action (set, get, delete, rotate)")
	cmd.Flags().StringVarP(&env, "env", "e", "", "Filter by environment")
	cmd.Flags().StringVar(&since, "since", "", "Filter entries after date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text, json, csv)")

	return cmd
}
