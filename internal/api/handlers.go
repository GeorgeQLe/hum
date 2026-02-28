package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// AppInfo is the summary returned by GET /api/status.
type AppInfo struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Command   string `json:"command"`
	Ports     []int  `json:"ports"`
	Status    string `json:"status"`
	PID       int    `json:"pid,omitempty"`
	Project   string `json:"project,omitempty"`
	Group     string `json:"group,omitempty"`
	AutoStart bool   `json:"autoStart,omitempty"`
}

// AppDetail is the full detail returned by GET /api/apps/{name}.
type AppDetail struct {
	AppInfo
	Uptime       string            `json:"uptime,omitempty"`
	RestartCount int               `json:"restartCount,omitempty"`
	ExitCode     int               `json:"exitCode,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	DependsOn    []string          `json:"dependsOn,omitempty"`
	ErrorCount   int               `json:"errorCount,omitempty"`
}

// LogEntry is a single log line.
type LogEntry struct {
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	IsStderr  bool      `json:"isStderr,omitempty"`
}

// ErrorEntry is a single captured error.
type ErrorEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Lines     []string  `json:"lines"`
	AppName   string    `json:"appName,omitempty"`
}

// PortMapping shows which app owns which port.
type PortMapping struct {
	Port    int    `json:"port"`
	AppName string `json:"appName"`
	Status  string `json:"status"`
}

// AppStats is resource usage for one app.
type AppStats struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	PID     int     `json:"pid,omitempty"`
	CPU     float64 `json:"cpu"`
	MemRSS  int64   `json:"memRss"`
	AvgCPU  float64 `json:"avgCpu,omitempty"`
	MaxCPU  float64 `json:"maxCpu,omitempty"`
	AvgMem  int64   `json:"avgMem,omitempty"`
	MaxMem  int64   `json:"maxMem,omitempty"`
	Uptime  string  `json:"uptime,omitempty"`
	Samples int     `json:"samples,omitempty"`
}

// Handler implements all REST endpoint handlers.
type Handler struct {
	deps ServerDeps
}

func newHandler(deps ServerDeps) *Handler {
	return &Handler{deps: deps}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b) //nolint:errcheck // crypto/rand.Read never fails
	return hex.EncodeToString(b)
}

// Health handles GET /api/health.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

// Status handles GET /api/status.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	apps := h.deps.GetApps()
	if apps == nil {
		apps = []AppInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"apps": apps,
	})
}

// AppDetail handles GET /api/apps/{name}.
func (h *Handler) AppDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}
	detail := h.deps.GetAppDetail(name)
	if detail == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", name))
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// AppLogs handles GET /api/apps/{name}/logs?lines=100.
func (h *Handler) AppLogs(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}

	lines := 100
	if q := r.URL.Query().Get("lines"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			lines = n
			if lines > 5000 {
				lines = 5000
			}
		}
	}

	logs := h.deps.GetLogs(name, lines)
	if logs == nil {
		logs = []LogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"app":   name,
		"lines": logs,
	})
}

// AppLogsStream handles GET /api/apps/{name}/logs/stream (SSE).
// Sends existing logs as an initial batch, then polls for new log lines
// and forwards them as SSE data frames in real time.
func (h *Handler) AppLogsStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Send existing logs as initial batch
	logs := h.deps.GetLogs(name, 5000)
	for _, l := range logs {
		data, _ := json.Marshal(l)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()
	sentCount := len(logs)

	// Poll for new logs and send keepalives
	ctx := r.Context()
	pollTicker := time.NewTicker(200 * time.Millisecond)
	defer pollTicker.Stop()
	keepaliveTicker := time.NewTicker(15 * time.Second)
	defer keepaliveTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			allLogs := h.deps.GetLogs(name, 5000)
			if len(allLogs) > sentCount {
				for _, l := range allLogs[sentCount:] {
					data, _ := json.Marshal(l)
					fmt.Fprintf(w, "data: %s\n\n", data)
				}
				flusher.Flush()
				sentCount = len(allLogs)
			}
		case <-keepaliveTicker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// AppErrors handles GET /api/apps/{name}/errors.
func (h *Handler) AppErrors(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}

	errors := h.deps.GetErrors(name)
	if errors == nil {
		errors = []ErrorEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"app":    name,
		"errors": errors,
	})
}

// Ports handles GET /api/ports.
func (h *Handler) Ports(w http.ResponseWriter, r *http.Request) {
	ports := h.deps.GetPorts()
	if ports == nil {
		ports = []PortMapping{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ports": ports,
	})
}

// Stats handles GET /api/stats.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	stats := h.deps.GetStats()
	if stats == nil {
		stats = []AppStats{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"apps": stats,
	})
}

// mutatingHandler is the common flow for mutating endpoints that require approval.
func (h *Handler) mutatingHandler(w http.ResponseWriter, r *http.Request, action, appName string, payload []byte) {
	clientName := r.Header.Get("X-Client-Name")
	if clientName == "" {
		clientName = "unknown"
	}

	// Check if approval is needed
	queue := h.deps.ApprovalQueue
	if queue != nil && queue.NeedsApproval(action) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = generateRequestID()
		}

		detail := appName
		if detail == "" && len(payload) > 0 {
			var p struct{ Name string `json:"name"` }
			json.Unmarshal(payload, &p) //nolint:errcheck // best-effort name extraction
			if p.Name != "" {
				detail = p.Name
			}
		}

		decision := queue.Submit(reqID, action, appName, clientName, detail)
		switch decision {
		case DecisionDenied:
			writeError(w, http.StatusForbidden, fmt.Sprintf("action %q denied by user", action))
			return
		case DecisionTimeout:
			writeError(w, http.StatusRequestTimeout, fmt.Sprintf("approval timeout for %q", action))
			return
		case DecisionSkipped:
			writeError(w, http.StatusForbidden, fmt.Sprintf("action %q skipped by user", action))
			return
		case DecisionApproved:
			// continue
		}
	}

	// Execute the action
	msg, err := h.deps.ExecuteAction(action, appName, payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"message": msg,
	})
}

// RegisterApp handles POST /api/apps.
func (h *Handler) RegisterApp(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read body")
		return
	}

	// Parse to validate structure
	var app struct {
		Name    string `json:"name"`
		Dir     string `json:"dir"`
		Command string `json:"command"`
		Ports   []int  `json:"ports"`
	}
	if err := json.Unmarshal(body, &app); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if app.Name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}

	h.mutatingHandler(w, r, "register", app.Name, body)
}

// RemoveApp handles DELETE /api/apps/{name}.
func (h *Handler) RemoveApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}
	h.mutatingHandler(w, r, "remove", name, nil)
}

// StartApp handles POST /api/apps/{name}/start.
func (h *Handler) StartApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}
	h.mutatingHandler(w, r, "start", name, nil)
}

// StopApp handles POST /api/apps/{name}/stop.
func (h *Handler) StopApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}
	h.mutatingHandler(w, r, "stop", name, nil)
}

// RestartApp handles POST /api/apps/{name}/restart.
func (h *Handler) RestartApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing app name")
		return
	}
	h.mutatingHandler(w, r, "restart", name, nil)
}

// ScanApps handles POST /api/apps/scan.
func (h *Handler) ScanApps(w http.ResponseWriter, r *http.Request) {
	h.mutatingHandler(w, r, "scan", "", nil)
}
