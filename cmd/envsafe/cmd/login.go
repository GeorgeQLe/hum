package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func LoginCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with envsafe server [EXPERIMENTAL — server not yet functional]",
		RunE: func(cmd *cobra.Command, args []string) error {
			warnIfInsecureHTTP(serverURL)

			fmt.Fprint(os.Stderr, "Email: ")
			reader := bufio.NewReader(os.Stdin)
			email, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading email: %w", err)
			}
			email = strings.TrimSpace(email)

			password, err := promptPassword("Password: ")
			if err != nil {
				return err
			}

			body, err := json.Marshal(map[string]string{
				"email":    email,
				"password": password,
			})
			if err != nil {
				return fmt.Errorf("encoding request: %w", err)
			}

			resp, err := http.Post(serverURL+"/api/auth/login", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("connecting to server: %w", err)
			}
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("reading response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("login failed: %s", string(respBody))
			}

			var result struct {
				Token   string `json:"token"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(respBody, &result); err != nil {
				return fmt.Errorf("parsing server response: %w", err)
			}

			// Store token in config file instead of printing to stdout
			configDir, err := os.UserConfigDir()
			if err != nil {
				return fmt.Errorf("finding config directory: %w", err)
			}
			tokenDir := filepath.Join(configDir, "envsafe")
			if err := os.MkdirAll(tokenDir, 0700); err != nil {
				return fmt.Errorf("creating config directory: %w", err)
			}
			tokenPath := filepath.Join(tokenDir, "token")
			if err := os.WriteFile(tokenPath, []byte(result.Token), 0600); err != nil {
				return fmt.Errorf("saving token: %w", err)
			}

			fmt.Fprintln(os.Stderr, "Logged in successfully.")
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8484", "Server URL")

	return cmd
}
