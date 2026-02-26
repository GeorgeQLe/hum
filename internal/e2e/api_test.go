//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/georgele/devctl/internal/process"
)

func TestAPIHealthNoAuth(t *testing.T) {
	sys := NewHeadlessSystem(t, nil)

	resp, err := sys.APIGetNoAuth("/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	body := ReadJSON(t, resp)
	if body["status"] != "ok" {
		t.Errorf("body.status: got %v, want ok", body["status"])
	}
}

func TestAPIAuthRequired(t *testing.T) {
	sys := NewHeadlessSystem(t, nil)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", sys.API.Port())

	// No auth header.
	resp, err := http.Get(baseURL + "/api/status")
	if err != nil {
		t.Fatalf("GET without auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no auth: got %d, want 401", resp.StatusCode)
	}

	// Wrong token.
	req, _ := http.NewRequest("GET", baseURL+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET with wrong token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong token: got %d, want 401", resp.StatusCode)
	}

	// Correct token.
	resp, err = sys.APIGet("/api/status")
	if err != nil {
		t.Fatalf("GET with correct token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("correct token: got %d, want 200", resp.StatusCode)
	}
}

func TestAPIStartStop(t *testing.T) {
	apps := []AppConfig{
		{Name: "worker", Command: `sh -c "echo running; sleep 30"`, Dir: "."},
	}
	sys := NewHeadlessSystem(t, apps)

	// Start the app via API.
	resp, err := sys.APIPost("/api/apps/worker/start", "")
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	body := ReadJSON(t, resp)
	if body["ok"] != true {
		t.Errorf("start response: %v", body)
	}

	WaitForStatus(t, sys.PM, "worker", process.StatusRunning, 5*time.Second)

	// GET status should show running.
	resp, err = sys.APIGet("/api/status")
	if err != nil {
		t.Fatalf("GET status: %v", err)
	}
	statusBody := ReadJSON(t, resp)
	appsArr, ok := statusBody["apps"].([]interface{})
	if !ok || len(appsArr) == 0 {
		t.Fatalf("expected apps array, got: %v", statusBody)
	}
	appMap := appsArr[0].(map[string]interface{})
	if appMap["status"] != "running" {
		t.Errorf("app status: got %v, want running", appMap["status"])
	}

	// Stop the app via API.
	resp, err = sys.APIPost("/api/apps/worker/stop", "")
	if err != nil {
		t.Fatalf("POST stop: %v", err)
	}
	body = ReadJSON(t, resp)
	if body["ok"] != true {
		t.Errorf("stop response: %v", body)
	}

	WaitForStatus(t, sys.PM, "worker", process.StatusStopped, 10*time.Second)

	// GET status should show stopped.
	resp, err = sys.APIGet("/api/status")
	if err != nil {
		t.Fatalf("GET status after stop: %v", err)
	}
	statusBody = ReadJSON(t, resp)
	appsArr = statusBody["apps"].([]interface{})
	appMap = appsArr[0].(map[string]interface{})
	if appMap["status"] != "stopped" {
		t.Errorf("app status after stop: got %v, want stopped", appMap["status"])
	}
}

func TestAPIAppDetail(t *testing.T) {
	apps := []AppConfig{
		{Name: "detail-app", Command: "sleep 30", Dir: ".", Ports: []int{3000}},
	}
	sys := NewHeadlessSystem(t, apps)

	// Known app returns 200.
	resp, err := sys.APIGet("/api/apps/detail-app")
	if err != nil {
		t.Fatalf("GET known app: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("known app: got %d, want 200", resp.StatusCode)
	}
	body := ReadJSON(t, resp)
	if body["name"] != "detail-app" {
		t.Errorf("name: got %v, want detail-app", body["name"])
	}
	if body["command"] != "sleep 30" {
		t.Errorf("command: got %v, want sleep 30", body["command"])
	}

	// Unknown app returns 404.
	resp, err = sys.APIGet("/api/apps/nonexistent")
	if err != nil {
		t.Fatalf("GET unknown app: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown app: got %d, want 404", resp.StatusCode)
	}
}

func TestAPILogs(t *testing.T) {
	apps := []AppConfig{
		{Name: "logger", Command: `sh -c "echo hello-from-logger; sleep 30"`, Dir: "."},
	}
	sys := NewHeadlessSystem(t, apps)

	// Start the app.
	resp, err := sys.APIPost("/api/apps/logger/start", "")
	if err != nil {
		t.Fatalf("POST start: %v", err)
	}
	resp.Body.Close()

	WaitForLogOutput(t, sys.PM, "logger", "hello-from-logger", 5*time.Second)

	// GET logs should return non-empty lines.
	resp, err = sys.APIGet("/api/apps/logger/logs")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	body := ReadJSON(t, resp)
	lines, ok := body["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected lines array, got: %v", body)
	}
	if len(lines) == 0 {
		t.Error("expected non-empty log lines")
	}

	// Verify at least one line contains our output.
	found := false
	for _, l := range lines {
		lineMap := l.(map[string]interface{})
		if text, ok := lineMap["text"].(string); ok {
			if text == "hello-from-logger\n" || text == "hello-from-logger" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("log output does not contain 'hello-from-logger'")
	}
}

func TestAPIPorts(t *testing.T) {
	apps := []AppConfig{
		{Name: "web", Command: "sleep 30", Dir: ".", Ports: []int{3000, 3001}},
		{Name: "api", Command: "sleep 30", Dir: ".", Ports: []int{8080}},
	}
	sys := NewHeadlessSystem(t, apps)

	resp, err := sys.APIGet("/api/ports")
	if err != nil {
		t.Fatalf("GET ports: %v", err)
	}
	body := ReadJSON(t, resp)
	ports, ok := body["ports"].([]interface{})
	if !ok {
		t.Fatalf("expected ports array, got: %v", body)
	}
	if len(ports) != 3 {
		t.Errorf("expected 3 port mappings, got %d", len(ports))
	}

	// Verify port mappings exist.
	portSet := make(map[float64]string)
	for _, p := range ports {
		pm := p.(map[string]interface{})
		portSet[pm["port"].(float64)] = pm["appName"].(string)
	}
	if portSet[3000] != "web" {
		t.Errorf("port 3000: got %q, want web", portSet[3000])
	}
	if portSet[3001] != "web" {
		t.Errorf("port 3001: got %q, want web", portSet[3001])
	}
	if portSet[8080] != "api" {
		t.Errorf("port 8080: got %q, want api", portSet[8080])
	}
}
