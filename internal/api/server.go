package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/georgele/devctl/internal/panicutil"
)

// Server is the HTTP API server embedded in the devctl process.
type Server struct {
	listener   net.Listener
	httpServer *http.Server
	token      string
	port       int
	handler    *Handler
}

// ServerDeps provides the dependencies the API server needs from the TUI/process layer.
type ServerDeps struct {
	// GetApps returns the current app list.
	GetApps func() []AppInfo
	// GetAppDetail returns detailed info for a single app.
	GetAppDetail func(name string) *AppDetail
	// GetLogs returns historical log lines for an app.
	GetLogs func(name string, lines int) []LogEntry
	// GetErrors returns error buffer for an app.
	GetErrors func(name string) []ErrorEntry
	// GetPorts returns the port allocation map.
	GetPorts func() []PortMapping
	// GetStats returns resource stats for all apps.
	GetStats func() []AppStats
	// ApprovalQueue for mutating operations.
	ApprovalQueue *ApprovalQueue
	// ExecuteAction performs a mutating action after approval.
	// Returns an error message if the action failed.
	ExecuteAction func(action, appName string, payload []byte) (string, error)
}

// NewServer creates and starts an HTTP API server on a dynamic port.
func NewServer(deps ServerDeps) (*Server, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("binding API server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	handler := newHandler(deps)
	mux := http.NewServeMux()

	// Read-only endpoints
	mux.HandleFunc("GET /api/health", handler.Health)
	mux.HandleFunc("GET /api/status", handler.Status)
	mux.HandleFunc("GET /api/apps/{name}", handler.AppDetail)
	mux.HandleFunc("GET /api/apps/{name}/logs", handler.AppLogs)
	mux.HandleFunc("GET /api/apps/{name}/logs/stream", handler.AppLogsStream)
	mux.HandleFunc("GET /api/apps/{name}/errors", handler.AppErrors)
	mux.HandleFunc("GET /api/ports", handler.Ports)
	mux.HandleFunc("GET /api/stats", handler.Stats)

	// Mutating endpoints
	mux.HandleFunc("POST /api/apps", handler.RegisterApp)
	mux.HandleFunc("DELETE /api/apps/{name}", handler.RemoveApp)
	mux.HandleFunc("POST /api/apps/{name}/start", handler.StartApp)
	mux.HandleFunc("POST /api/apps/{name}/stop", handler.StopApp)
	mux.HandleFunc("POST /api/apps/{name}/restart", handler.RestartApp)
	mux.HandleFunc("POST /api/apps/scan", handler.ScanApps)

	// Wrap with auth middleware
	authed := authMiddleware(token, mux)

	srv := &http.Server{
		Handler:      authed,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second, // long for approval blocking
		IdleTimeout:  120 * time.Second,
	}

	s := &Server{
		listener:   listener,
		httpServer: srv,
		token:      token,
		port:       port,
		handler:    handler,
	}

	// Write discovery file
	if err := WriteDiscovery(DiscoveryInfo{
		PID:   os.Getpid(),
		Port:  port,
		Token: token,
	}); err != nil {
		listener.Close()
		return nil, fmt.Errorf("writing discovery: %w", err)
	}

	// Start serving
	go func() {
		defer panicutil.Recover("api server")
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("API server error: %v", err)
		}
	}()

	return s, nil
}

// Port returns the port the server is listening on.
func (s *Server) Port() int {
	return s.port
}

// Token returns the auth token.
func (s *Server) Token() string {
	return s.token
}

// Stop gracefully shuts down the server and removes the discovery file.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.httpServer.Shutdown(ctx) //nolint:errcheck // best-effort shutdown
	RemoveDiscovery()
}

// authMiddleware checks for a valid Bearer token on all requests except /api/health.
func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health endpoint is unauthenticated
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization")
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		next.ServeHTTP(w, r)
	})
}
