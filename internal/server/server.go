package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Config holds server configuration.
type Config struct {
	Addr       string
	DBConnStr  string
	JWTSecret  string
	TLSCert    string
	TLSKey     string
}

// Server is the envsafe HTTP server.
type Server struct {
	config Config
	router *http.ServeMux
	server *http.Server
}

// New creates a new envsafe server.
func New(cfg Config) *Server {
	s := &Server{
		config: cfg,
		router: http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:         s.config.Addr,
		Handler:      s.withMiddleware(s.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("envsafe server listening on %s", s.config.Addr)

	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		return s.server.ListenAndServeTLS(s.config.TLSCert, s.config.TLSKey)
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Request logging
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func (s *Server) registerRoutes() {
	// Health check
	s.router.HandleFunc("GET /api/health", s.handleHealth)

	// Auth
	s.router.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.router.HandleFunc("POST /api/auth/login", s.handleLogin)
	s.router.HandleFunc("POST /api/auth/totp/setup", s.handleTOTPSetup)
	s.router.HandleFunc("POST /api/auth/totp/verify", s.handleTOTPVerify)

	// Secrets
	s.router.HandleFunc("GET /api/secrets/{env}", s.handleListSecrets)
	s.router.HandleFunc("GET /api/secrets/{env}/{key}", s.handleGetSecret)
	s.router.HandleFunc("PUT /api/secrets/{env}/{key}", s.handleSetSecret)
	s.router.HandleFunc("DELETE /api/secrets/{env}/{key}", s.handleDeleteSecret)

	// Environments
	s.router.HandleFunc("GET /api/environments", s.handleListEnvironments)
	s.router.HandleFunc("POST /api/environments", s.handleCreateEnvironment)

	// Users (admin)
	s.router.HandleFunc("GET /api/users", s.handleListUsers)
	s.router.HandleFunc("PUT /api/users/{id}/role", s.handleSetUserRole)

	// Audit
	s.router.HandleFunc("GET /api/audit", s.handleAuditLog)

	// Share
	s.router.HandleFunc("POST /api/share", s.handleCreateShare)
	s.router.HandleFunc("GET /api/share/{token}", s.handleGetShare)
}

// JSON helper
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

func jsonOK(w http.ResponseWriter, data string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, data)
}
