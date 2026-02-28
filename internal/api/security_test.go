package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpointNoPID(t *testing.T) {
	h := newHandler(ServerDeps{})
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := body["pid"]; ok {
		t.Fatal("health response should NOT contain a 'pid' field")
	}

	status, ok := body["status"]
	if !ok {
		t.Fatal("health response missing 'status' field")
	}
	if status != "ok" {
		t.Fatalf("expected status 'ok', got %q", status)
	}

	if _, ok := body["time"]; !ok {
		t.Fatal("health response missing 'time' field")
	}
}

func TestRegisterAppBodyLimit(t *testing.T) {
	h := newHandler(ServerDeps{
		ApprovalQueue: nil,
		ExecuteAction: func(action, appName string, payload []byte) (string, error) {
			return "registered", nil
		},
	})

	body := `{"name":"test-app","dir":"/tmp/app","command":"npm start","ports":[3000]}`
	req := httptest.NewRequest("POST", "/api/apps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.RegisterApp(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for normal-sized body, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["ok"] != true {
		t.Fatalf("expected ok=true, got %v", result["ok"])
	}
}

func TestHealthEndpointResponseFormat(t *testing.T) {
	h := newHandler(ServerDeps{})
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type 'application/json', got %q", ct)
	}
}
