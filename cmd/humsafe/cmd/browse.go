package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/georgele/hum/internal/vault"
	"github.com/georgele/hum/internal/vault/audit"

	humsafetui "github.com/georgele/hum/internal/humsafe-tui"
)

func BrowseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "browse",
		Short: "Open interactive TUI browser",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
			}

			if !vault.Exists(projectRoot) {
				return fmt.Errorf("no vault found. Run 'humsafe init' first")
			}

			v, err := openAndUnlock(projectRoot)
			if err != nil {
				return err
			}
			defer v.Lock()

			logger := audit.NewLogger(vault.VaultPath(projectRoot))
			model := humsafetui.New(v, logger)

			p := tea.NewProgram(model, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("TUI error: %w", err)
			}

			return nil
		},
	}
}
