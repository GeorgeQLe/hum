package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/georgele/hum/internal/vault"
	"github.com/spf13/cobra"
)

func ShareCmd() *cobra.Command {
	var env string
	var serverURL string
	var expiresIn int
	var insecure bool

	cmd := &cobra.Command{
		Use:   "share <key>",
		Short: "Create a one-time share link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !insecure {
				u, err := url.Parse(serverURL)
				if err == nil {
					host := u.Hostname()
					if host != "localhost" && host != "127.0.0.1" && host != "::1" && !strings.HasPrefix(serverURL, "https://") {
						return fmt.Errorf("refusing to send secrets over unencrypted HTTP to non-local server %q (use --insecure to override)", serverURL)
					}
				}
			}

			if expiresIn <= 0 || expiresIn > 604800 {
				return fmt.Errorf("--expires must be between 1 and 604800 seconds (7 days)")
			}

			key := args[0]

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

			value, err := v.Get(env, key)
			if err != nil {
				return err
			}

			body, err := json.Marshal(map[string]interface{}{
				"environment": env,
				"key":         key,
				"value":       value,
				"expires_in":  expiresIn,
			})
			if err != nil {
				return fmt.Errorf("encoding request: %w", err)
			}

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Post(serverURL+"/api/share", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("connecting to server: %w", err)
			}
			defer resp.Body.Close()

			const maxResponseSize = 1 << 20 // 1 MB
			respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("share failed: %s", string(respBody))
			}

			var result struct {
				Token     string `json:"token"`
				ExpiresIn int    `json:"expires_in"`
			}
			if err := json.Unmarshal(respBody, &result); err != nil {
				return fmt.Errorf("parsing server response: %w", err)
			}

			fmt.Printf("Share link: %s/api/share/%s\n", serverURL, result.Token)
			fmt.Printf("Expires in: %d seconds\n", result.ExpiresIn)
			fmt.Println("This link can only be used once.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Source environment")
	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8484", "Server URL")
	cmd.Flags().IntVar(&expiresIn, "expires", 3600, "Link expiry in seconds")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "Allow HTTP with non-local servers")

	return cmd
}
