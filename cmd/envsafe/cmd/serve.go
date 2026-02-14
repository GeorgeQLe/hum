package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/georgele/devctl/internal/server"
	"github.com/spf13/cobra"
)

func ServeCmd() *cobra.Command {
	var addr string
	var tlsCert string
	var tlsKey string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the envsafe server",
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return srv.Shutdown(context.Background())
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8484", "Server listen address")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate file")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS key file")

	return cmd
}
