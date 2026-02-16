package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/georgele/devctl/internal/config"
	"github.com/georgele/devctl/internal/tui"
)

func newRootCmd() *cobra.Command {
	var startAll bool
	var restore bool

	cmd := &cobra.Command{
		Use:   "devctl",
		Short: "Multi-app dev server manager",
		Long:  "devctl — A TUI for managing multiple local development servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(startAll, restore)
		},
	}

	cmd.Flags().BoolVar(&startAll, "start-all", false, "Start all apps on launch")
	cmd.Flags().BoolVar(&restore, "restore", false, "Restore previous session")

	return cmd
}

func runTUI(startAll, restore bool) error {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("could not find project root: %w", err)
	}

	apps, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	model := tui.New(projectRoot, apps, startAll, restore)

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	// Handle SIGTERM/SIGHUP — forward to Bubble Tea as an interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	// Handle SIGUSR1 — dev reload (graceful restart without killing managed apps)
	devReloadCh := make(chan os.Signal, 1)
	signal.Notify(devReloadCh, syscall.SIGUSR1)
	defer signal.Stop(devReloadCh)

	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			defer func() { recover() }()
			p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
		case <-devReloadCh:
			defer func() { recover() }()
			p.Send(tui.DevReloadMsg{})
		case <-done:
			return
		}
	}()
	defer close(done)

	// Panic recovery to restore terminal state
	defer func() {
		if r := recover(); r != nil {
			p.Kill()
			fmt.Fprintf(os.Stderr, "devctl panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
