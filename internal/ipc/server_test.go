package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPingRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()
	server.Start()

	// Respond to requests in background
	go func() {
		for req := range server.Requests() {
			if req.Request.Action == "ping" {
				req.ResponseCh <- Response{
					OK:      true,
					PID:     os.Getpid(),
					Project: projectRoot,
				}
			}
		}
	}()

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	client := NewClient(projectRoot)
	resp, err := client.Ping()
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true, got false")
	}
	if resp.PID != os.Getpid() {
		t.Errorf("expected PID=%d, got %d", os.Getpid(), resp.PID)
	}
}

func TestStatusRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()
	server.Start()

	type appStatus struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	go func() {
		for req := range server.Requests() {
			if req.Request.Action == "status" {
				apps := []appStatus{{Name: "test-app", Status: "running"}}
				appsJSON, _ := json.Marshal(apps)
				req.ResponseCh <- Response{
					OK:      true,
					PID:     os.Getpid(),
					Project: projectRoot,
					Apps:    appsJSON,
				}
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(projectRoot)
	resp, err := client.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true")
	}
	if resp.Apps == nil {
		t.Fatal("expected apps in response")
	}

	var apps []appStatus
	if err := json.Unmarshal(resp.Apps, &apps); err != nil {
		t.Fatalf("unmarshal apps: %v", err)
	}
	if len(apps) != 1 || apps[0].Name != "test-app" {
		t.Errorf("unexpected apps: %v", apps)
	}
}

func TestAddAppRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()
	server.Start()

	go func() {
		for req := range server.Requests() {
			if req.Request.Action == "add-app" {
				req.ResponseCh <- Response{
					OK:      true,
					Name:    "new-app",
					Message: "Added new-app",
				}
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(projectRoot)
	appData, _ := json.Marshal(map[string]interface{}{
		"name":    "new-app",
		"dir":     "apps/new-app",
		"command": "npm run dev",
		"ports":   []int{3001},
	})

	resp, err := client.AddApp(appData, projectRoot, false)
	if err != nil {
		t.Fatalf("AddApp: %v", err)
	}
	if !resp.OK {
		t.Errorf("expected OK=true")
	}
	if resp.Name != "new-app" {
		t.Errorf("expected name='new-app', got %q", resp.Name)
	}
}

func TestStaleSocketCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	// Create a stale socket file
	socketPath := SocketPath(projectRoot)
	os.MkdirAll(filepath.Dir(socketPath), 0755)
	os.WriteFile(socketPath, []byte("stale"), 0600)

	// Should be able to create server even with stale socket
	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer should handle stale socket: %v", err)
	}
	server.Stop()
}

func TestInvalidJSONRequest(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()
	server.Start()

	time.Sleep(50 * time.Millisecond)

	// Send invalid JSON directly via the socket
	conn, err := net.DialTimeout("unix", server.socketPath, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send garbage
	conn.Write([]byte("not valid json\n"))

	// Should get an error response
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatal("expected response for invalid JSON")
	}

	var resp Response
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.OK {
		t.Error("expected OK=false for invalid JSON")
	}
	if resp.Error == "" {
		t.Error("expected error message for invalid JSON")
	}
}

func TestConcurrentClients(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()
	server.Start()

	// Respond to all requests
	go func() {
		for req := range server.Requests() {
			req.ResponseCh <- Response{OK: true, PID: os.Getpid()}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// Launch multiple concurrent clients
	const numClients = 10
	errCh := make(chan error, numClients)

	for i := 0; i < numClients; i++ {
		go func() {
			client := NewClient(projectRoot)
			resp, err := client.Ping()
			if err != nil {
				errCh <- err
				return
			}
			if !resp.OK {
				errCh <- fmt.Errorf("expected OK=true")
				return
			}
			errCh <- nil
		}()
	}

	for i := 0; i < numClients; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("client %d: %v", i, err)
		}
	}
}

func TestSocketPathDeterministic(t *testing.T) {
	path1 := SocketPath("/some/project")
	path2 := SocketPath("/some/project")
	if path1 != path2 {
		t.Errorf("SocketPath not deterministic: %q != %q", path1, path2)
	}

	path3 := SocketPath("/other/project")
	if path1 == path3 {
		t.Error("different projects should have different socket paths")
	}
}
