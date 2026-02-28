package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64 char token, got %d", len(token))
	}
	// Tokens should be unique
	token2, _ := GenerateToken()
	if token == token2 {
		t.Fatal("tokens should be unique")
	}
}

func TestApprovalQueue(t *testing.T) {
	cfg := ApprovalConfig{
		TimeoutSeconds: 2,
		Rules: map[string]ApprovalRule{
			"start": ApprovalRequired,
			"stop":  ApprovalAuto,
		},
	}
	q := NewApprovalQueue(cfg)

	// Auto actions don't need approval
	if q.NeedsApproval("stop") {
		t.Fatal("stop should be auto")
	}
	if !q.NeedsApproval("start") {
		t.Fatal("start should require approval")
	}
	if !q.NeedsApproval("unknown") {
		t.Fatal("unknown actions should default to required")
	}

	// Test approve flow
	done := make(chan ApprovalDecision, 1)
	go func() {
		d := q.Submit("req-1", "start", "myapp", "test-client", "myapp")
		done <- d
	}()

	time.Sleep(50 * time.Millisecond)
	if q.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", q.PendingCount())
	}

	pending := q.Pending()
	if pending[0].ID != "req-1" {
		t.Fatal("wrong request ID")
	}

	q.Decide(DecisionApproved)

	d := <-done
	if d != DecisionApproved {
		t.Fatalf("expected approved, got %d", d)
	}
}

func TestApprovalTimeout(t *testing.T) {
	cfg := ApprovalConfig{
		TimeoutSeconds: 1,
		Rules: map[string]ApprovalRule{
			"start": ApprovalRequired,
		},
	}
	q := NewApprovalQueue(cfg)

	start := time.Now()
	d := q.Submit("req-timeout", "start", "myapp", "test", "myapp")
	elapsed := time.Since(start)

	if d != DecisionTimeout {
		t.Fatalf("expected timeout, got %d", d)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("timeout too fast: %v", elapsed)
	}
}

func TestHTTPServer(t *testing.T) {
	deps := ServerDeps{
		GetApps: func() []AppInfo {
			return []AppInfo{
				{Name: "test-app", Status: "stopped", Ports: []int{3000}},
			}
		},
		GetAppDetail: func(name string) *AppDetail {
			if name == "test-app" {
				return &AppDetail{
					AppInfo: AppInfo{Name: "test-app", Status: "stopped", Ports: []int{3000}},
				}
			}
			return nil
		},
		GetLogs: func(name string, lines int) []LogEntry {
			return []LogEntry{{Text: "hello", Timestamp: time.Now()}}
		},
		GetErrors: func(name string) []ErrorEntry { return nil },
		GetPorts: func() []PortMapping {
			return []PortMapping{{Port: 3000, AppName: "test-app", Status: "stopped"}}
		},
		GetStats: func() []AppStats {
			return []AppStats{{Name: "test-app", Status: "stopped"}}
		},
		ApprovalQueue: nil, // no approval for this test
		ExecuteAction: func(action, appName string, payload []byte) (string, error) {
			return "done", nil
		},
	}

	srv, err := NewServer(deps)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	baseURL := "http://127.0.0.1:" + strings.Split(srv.listener.Addr().String(), ":")[1]
	token := srv.Token()

	// Test unauthenticated health endpoint
	resp, err := http.Get(baseURL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test authenticated status endpoint
	req, _ := http.NewRequest("GET", baseURL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var statusResp struct {
		Apps []AppInfo `json:"apps"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &statusResp); err != nil {
		t.Fatal(err)
	}
	if len(statusResp.Apps) != 1 || statusResp.Apps[0].Name != "test-app" {
		t.Fatalf("unexpected status response: %s", body)
	}

	// Test unauthorized request
	req2, _ := http.NewRequest("GET", baseURL+"/api/status", nil)
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Test app detail
	req3, _ := http.NewRequest("GET", baseURL+"/api/apps/test-app", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	resp3, _ := http.DefaultClient.Do(req3)
	if resp3.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	resp3.Body.Close()

	// Test 404 for unknown app
	req4, _ := http.NewRequest("GET", baseURL+"/api/apps/nonexistent", nil)
	req4.Header.Set("Authorization", "Bearer "+token)
	resp4, _ := http.DefaultClient.Do(req4)
	if resp4.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp4.StatusCode)
	}
	resp4.Body.Close()

	// Test mutating endpoint (no approval queue, should execute directly)
	req5, _ := http.NewRequest("POST", baseURL+"/api/apps/test-app/start", nil)
	req5.Header.Set("Authorization", "Bearer "+token)
	resp5, _ := http.DefaultClient.Do(req5)
	if resp5.StatusCode != 200 {
		body5, _ := io.ReadAll(resp5.Body)
		t.Fatalf("expected 200, got %d: %s", resp5.StatusCode, body5)
	}
	resp5.Body.Close()
}

func TestAPIAuthRequired(t *testing.T) {
	deps := ServerDeps{
		GetApps:       func() []AppInfo { return nil },
		GetAppDetail:  func(name string) *AppDetail { return nil },
		GetLogs:       func(name string, lines int) []LogEntry { return nil },
		GetErrors:     func(name string) []ErrorEntry { return nil },
		GetPorts:      func() []PortMapping { return nil },
		GetStats:      func() []AppStats { return nil },
		ApprovalQueue: nil,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) { return "", nil },
	}

	srv, err := NewServer(deps)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	baseURL := "http://127.0.0.1:" + strings.Split(srv.listener.Addr().String(), ":")[1]

	// Test all authenticated endpoints reject without token
	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/status"},
		{"GET", "/api/apps/test"},
		{"GET", "/api/apps/test/logs"},
		{"GET", "/api/apps/test/errors"},
		{"GET", "/api/ports"},
		{"GET", "/api/stats"},
		{"POST", "/api/apps/test/start"},
		{"POST", "/api/apps/test/stop"},
		{"POST", "/api/apps/test/restart"},
		{"DELETE", "/api/apps/test"},
	}

	for _, ep := range endpoints {
		req, _ := http.NewRequest(ep.method, baseURL+ep.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", ep.method, ep.path, err)
		}
		if resp.StatusCode != 401 {
			t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Test with wrong token
	req, _ := http.NewRequest("GET", baseURL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 401 {
		t.Errorf("wrong token: expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPIInputValidation(t *testing.T) {
	deps := ServerDeps{
		GetApps:       func() []AppInfo { return nil },
		GetAppDetail:  func(name string) *AppDetail { return nil },
		GetLogs:       func(name string, lines int) []LogEntry { return nil },
		GetErrors:     func(name string) []ErrorEntry { return nil },
		GetPorts:      func() []PortMapping { return nil },
		GetStats:      func() []AppStats { return nil },
		ApprovalQueue: nil,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) {
			return "done", nil
		},
	}

	srv, err := NewServer(deps)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	baseURL := "http://127.0.0.1:" + strings.Split(srv.listener.Addr().String(), ":")[1]
	token := srv.Token()

	// Test registering app with no name
	req, _ := http.NewRequest("POST", baseURL+"/api/apps", strings.NewReader(`{"dir":"apps/test","command":"npm start"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != 400 {
		t.Errorf("register without name: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test registering app with invalid JSON
	req2, _ := http.NewRequest("POST", baseURL+"/api/apps", strings.NewReader(`not json`))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2.StatusCode != 400 {
		t.Errorf("register with invalid JSON: expected 400, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

func TestRateLimiting(t *testing.T) {
	deps := ServerDeps{
		GetApps:       func() []AppInfo { return []AppInfo{} },
		GetAppDetail:  func(name string) *AppDetail { return nil },
		GetLogs:       func(name string, lines int) []LogEntry { return nil },
		GetErrors:     func(name string) []ErrorEntry { return nil },
		GetPorts:      func() []PortMapping { return nil },
		GetStats:      func() []AppStats { return nil },
		ApprovalQueue: nil,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) { return "", nil },
	}

	srv, err := NewServer(deps)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	baseURL := "http://127.0.0.1:" + strings.Split(srv.listener.Addr().String(), ":")[1]
	token := srv.Token()

	// Send 150 requests rapidly — some should be rate-limited
	rateLimited := 0
	for i := 0; i < 150; i++ {
		req, _ := http.NewRequest("GET", baseURL+"/api/status", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if resp.StatusCode == 429 {
			rateLimited++
		}
		resp.Body.Close()
	}

	if rateLimited == 0 {
		t.Error("expected some requests to be rate-limited after 150 rapid requests")
	}
}

func TestPIDFileLocation(t *testing.T) {
	dir := GlobalDir()
	// GlobalDir should NOT fall back to shared /tmp/.humrun
	sharedTmp := filepath.Join(os.TempDir(), ".humrun")
	if dir == sharedTmp {
		t.Errorf("GlobalDir() = %q, should not be shared /tmp path", dir)
	}

	// Should typically be under user's home directory
	home, err := os.UserHomeDir()
	if err == nil {
		if !strings.HasPrefix(dir, home) {
			t.Errorf("GlobalDir() = %q, should be under user home %q", dir, home)
		}
	}
}

func TestAPIBodySizeLimit(t *testing.T) {
	deps := ServerDeps{
		GetApps:       func() []AppInfo { return nil },
		GetAppDetail:  func(name string) *AppDetail { return nil },
		GetLogs:       func(name string, lines int) []LogEntry { return nil },
		GetErrors:     func(name string) []ErrorEntry { return nil },
		GetPorts:      func() []PortMapping { return nil },
		GetStats:      func() []AppStats { return nil },
		ApprovalQueue: nil,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) { return "", nil },
	}

	srv, err := NewServer(deps)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	baseURL := "http://127.0.0.1:" + strings.Split(srv.listener.Addr().String(), ":")[1]
	token := srv.Token()

	// Send a body larger than 1MB — should be rejected
	largeBody := strings.Repeat("x", 2*1024*1024)
	req, _ := http.NewRequest("POST", baseURL+"/api/apps", strings.NewReader(largeBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// http.MaxBytesReader returns 413 or the handler returns 400 when Read fails
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized body: expected 413 or 400, got %d", resp.StatusCode)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Approval.TimeoutSeconds != 60 {
		t.Fatalf("expected timeout 60, got %d", cfg.Approval.TimeoutSeconds)
	}
	if cfg.Approval.Rules["register"] != ApprovalRequired {
		t.Fatal("register should be required by default")
	}
}
