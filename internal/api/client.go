package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for the humrun API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClientFromDiscovery creates a client by reading ~/.humrun/api.json.
func NewClientFromDiscovery() (*Client, error) {
	info, err := ReadDiscovery()
	if err != nil {
		return nil, fmt.Errorf("humrun API not available: %w", err)
	}
	return &Client{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", info.Port),
		Token:   info.Token,
		HTTP: &http.Client{
			Timeout: 90 * time.Second, // long for approval blocking
		},
	}, nil
}

// do performs an authenticated request and returns the response body.
func (c *Client) do(method, path string, body interface{}) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Name", "humrun-cli")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// Health checks the API server health.
func (c *Client) Health() error {
	_, status, err := c.do("GET", "/api/health", nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("unhealthy: status %d", status)
	}
	return nil
}

// Status returns all app statuses.
func (c *Client) Status() ([]byte, error) {
	data, status, err := c.do("GET", "/api/status", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, data)
	}
	return data, nil
}

// AppDetail returns details for a single app.
func (c *Client) AppDetail(name string) ([]byte, error) {
	data, status, err := c.do("GET", "/api/apps/"+name, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, data)
	}
	return data, nil
}

// AppLogs returns log lines for an app.
func (c *Client) AppLogs(name string, lines int) ([]byte, error) {
	path := fmt.Sprintf("/api/apps/%s/logs?lines=%d", name, lines)
	data, status, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, data)
	}
	return data, nil
}

// AppErrors returns errors for an app.
func (c *Client) AppErrors(name string) ([]byte, error) {
	data, status, err := c.do("GET", "/api/apps/"+name+"/errors", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, data)
	}
	return data, nil
}

// Ports returns the port allocation map.
func (c *Client) Ports() ([]byte, error) {
	data, status, err := c.do("GET", "/api/ports", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, data)
	}
	return data, nil
}

// Stats returns resource usage stats.
func (c *Client) Stats() ([]byte, error) {
	data, status, err := c.do("GET", "/api/stats", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("status %d: %s", status, data)
	}
	return data, nil
}

// RegisterApp registers a new app via the API.
func (c *Client) RegisterApp(app interface{}) ([]byte, error) {
	data, status, err := c.do("POST", "/api/apps", app)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, parseAPIError(data, status)
	}
	return data, nil
}

// RemoveApp removes an app via the API.
func (c *Client) RemoveApp(name string) ([]byte, error) {
	data, status, err := c.do("DELETE", "/api/apps/"+name, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, parseAPIError(data, status)
	}
	return data, nil
}

// StartApp starts an app via the API.
func (c *Client) StartApp(name string) ([]byte, error) {
	data, status, err := c.do("POST", "/api/apps/"+name+"/start", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, parseAPIError(data, status)
	}
	return data, nil
}

// StopApp stops an app via the API.
func (c *Client) StopApp(name string) ([]byte, error) {
	data, status, err := c.do("POST", "/api/apps/"+name+"/stop", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, parseAPIError(data, status)
	}
	return data, nil
}

// RestartApp restarts an app via the API.
func (c *Client) RestartApp(name string) ([]byte, error) {
	data, status, err := c.do("POST", "/api/apps/"+name+"/restart", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, parseAPIError(data, status)
	}
	return data, nil
}

// ScanApps triggers an app scan via the API.
func (c *Client) ScanApps() ([]byte, error) {
	data, status, err := c.do("POST", "/api/apps/scan", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, parseAPIError(data, status)
	}
	return data, nil
}

func parseAPIError(data []byte, status int) error {
	var errResp struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("%s", errResp.Error)
	}
	return fmt.Errorf("API error (status %d): %s", status, data)
}
