package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Client connects to a running devctl IPC server.
type Client struct {
	socketPath string
}

// NewClient creates a new IPC client for the given project root.
func NewClient(projectRoot string) *Client {
	return &Client{
		socketPath: SocketPath(projectRoot),
	}
}

// Send sends a request and returns the response.
func (c *Client) Send(req Request) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not connect to devctl: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	if _, err := conn.Write(data); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no response from server")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}

	return &resp, nil
}

// Ping checks if the devctl server is running.
func (c *Client) Ping() (*Response, error) {
	return c.Send(Request{Action: "ping"})
}

// Status returns the status of all apps.
func (c *Client) Status() (*Response, error) {
	return c.Send(Request{Action: "status"})
}

// AddApp adds an app to the running devctl instance.
func (c *Client) AddApp(appJSON json.RawMessage, cwd string, autoStart bool) (*Response, error) {
	return c.Send(Request{
		Action:    "add-app",
		App:       appJSON,
		Cwd:       cwd,
		AutoStart: autoStart,
	})
}

// Stats returns resource statistics for all apps.
func (c *Client) Stats() (*Response, error) {
	return c.Send(Request{Action: "stats"})
}
