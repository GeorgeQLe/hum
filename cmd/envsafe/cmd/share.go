package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/georgele/devctl/internal/vault"
	"github.com/spf13/cobra"
)

func ShareCmd() *cobra.Command {
	var env string
	var serverURL string
	var expiresIn int

	cmd := &cobra.Command{
		Use:   "share <key>",
		Short: "Create a one-time share link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			projectRoot, err := findProjectRoot()
			if err != nil {
				return err
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

			body, _ := json.Marshal(map[string]interface{}{
				"environment": env,
				"key":         key,
				"value":       value,
				"expires_in":  expiresIn,
			})

			resp, err := http.Post(serverURL+"/api/share", "application/json", bytes.NewReader(body))
			if err != nil {
				return fmt.Errorf("connecting to server: %w", err)
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("share failed: %s", string(respBody))
			}

			var result struct {
				Token     string `json:"token"`
				ExpiresIn int    `json:"expires_in"`
			}
			json.Unmarshal(respBody, &result)

			fmt.Printf("Share link: %s/api/share/%s\n", serverURL, result.Token)
			fmt.Printf("Expires in: %d seconds\n", result.ExpiresIn)
			fmt.Println("This link can only be used once.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&env, "env", "e", vault.DefaultEnv, "Source environment")
	cmd.Flags().StringVar(&serverURL, "server", "http://localhost:8484", "Server URL")
	cmd.Flags().IntVar(&expiresIn, "expires", 3600, "Link expiry in seconds")

	return cmd
}
