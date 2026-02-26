package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/georgele/devctl/internal/server"
	"github.com/spf13/cobra"
)

func ServeCmd() *cobra.Command {
	var addr string
	var tlsCert string
	var tlsKey string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the envsafe server [EXPERIMENTAL — not yet functional]",
		Long: `Start the envsafe server.

WARNING: The server is experimental and not yet functional.
All API endpoints currently return 501 Not Implemented.
Use the local vault commands (init, set, get, list, etc.) for production use.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "WARNING: The envsafe server is experimental. All API endpoints return 501 Not Implemented.")
			fmt.Fprintln(os.Stderr, "Use local vault commands for production workflows.")
			fmt.Fprintln(os.Stderr, "")
			cfg := server.Config{
				Addr:    addr,
				TLSCert: tlsCert,
				TLSKey:  tlsKey,
			}

			srv := server.New(cfg)

			// Graceful shutdown on SIGINT/SIGTERM
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			errCh := make(chan error, 1)
			go func() {
				errCh <- srv.Start()
			}()

			select {
			case err := <-errCh:
				return fmt.Errorf("server error: %w", err)
			case sig := <-sigCh:
				fmt.Printf("\nReceived %s, shutting down...\n", sig)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				return srv.Shutdown(ctx)
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8484", "Server listen address")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate file")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS key file")
	cmd.MarkFlagsRequiredTogether("tls-cert", "tls-key")

	return cmd
}
