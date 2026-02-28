package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestSocketPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()

	// Verify socket file has 0600 permissions
	info, err := os.Stat(server.socketPath)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("socket permissions = %o, want 0600", perm)
	}
}

func TestIPCPathTraversal(t *testing.T) {
	// Verify SocketPath is safe with path traversal attempts
	path1 := SocketPath("/some/../etc/passwd")
	path2 := SocketPath("/some/project")

	// Both should be in the socket directory, not escape it
	if !filepath.IsAbs(path1) {
		t.Error("socket path should be absolute")
	}
	if filepath.Dir(path1) != socketDir() {
		t.Errorf("socket path escaped socket dir: %q", path1)
	}
	if filepath.Dir(path2) != socketDir() {
		t.Errorf("socket path escaped socket dir: %q", path2)
	}

	// Path traversal in project root should produce different (safe) hashes
	if path1 == path2 {
		t.Error("different project roots should produce different socket paths")
	}
}

func TestSocketDirOwnership(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	os.MkdirAll(projectRoot, 0755)

	server, err := NewServer(projectRoot)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Stop()

	// Verify socket directory has 0700 permissions
	dir := filepath.Dir(server.socketPath)
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat socket dir: %v", err)
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		t.Errorf("socket dir permissions = %o, group/other should have no access", perm)
	}
}

func TestIPCMessageSizeLimit(t *testing.T) {
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

	// Send a message larger than maxIPCMessageSize (64KB)
	conn, err := net.DialTimeout("unix", server.socketPath, 2*time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Create an oversized message (>64KB) as a single line
	oversized := strings.Repeat("x", maxIPCMessageSize+1024) + "\n"
	conn.Write([]byte(oversized))

	// The scanner should reject this — the connection should close or return an error response
	scanner := bufio.NewScanner(conn)
	// If scanner.Scan() returns true, the server managed to read it (shouldn't happen with oversized)
	// If it returns false, the connection was closed or errored (expected behavior)
	if scanner.Scan() {
		// If we got a response, it should be an error
		var resp Response
		if err := json.Unmarshal(scanner.Bytes(), &resp); err == nil && resp.OK {
			t.Error("oversized message should not succeed")
		}
	}
	// If scanner.Scan() returns false, that's the expected behavior — connection closed
}

func TestSocketDirNotSharedTmp(t *testing.T) {
	dir := socketDir()
	// Socket dir should NOT be directly in /tmp (shared temp space)
	if dir == filepath.Join(os.TempDir(), "humrun-sockets") {
		t.Errorf("socketDir() = %q, should not be in shared /tmp", dir)
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
