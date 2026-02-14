package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

func LoginCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with envsafe server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print("Email: ")
			var email string
			fmt.Scanln(&email)

			password, err := promptPassword("Password: ")
			if err != nil {
				return err
			}

			body, _ := json.Marshal(map[string]string{
				"email":    email,
				"password": password,
			})

			resp, err := http.Post(serverURL+"/api/auth/login", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("connecting to server: %w", err)
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("login failed: %s", string(respBody))
			}

			var result struct {
				Token   string `json:"token"`
				Message string `json:"message"`
			}
			json.Unmarshal(respBody, &result)

			fmt.Printf("Logged in successfully. Token: %s\n", result.Token)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8484", "Server URL")

	return cmd
}
