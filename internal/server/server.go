package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/georgele/hum/internal/server/auth"
)

// Config holds server configuration.
type Config struct {
	Addr           string
	DBConnStr      string
	JWTSecret      string
	TLSCert        string
	TLSKey         string
	AllowedOrigins []string
}

// Server is the humsafe HTTP server.
type Server struct {
	config Config
	router *http.ServeMux
	server *http.Server
}

// New creates a new humsafe server.
func New(cfg Config) *Server {
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{"http://localhost:5173"}
	}
	if cfg.JWTSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("failed to generate random JWT secret: %v", err)
		}
		cfg.JWTSecret = hex.EncodeToString(b)
		log.Println("WARNING: No --jwt-secret provided; using a randomly generated secret (tokens will not survive restarts)")
	}
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

	log.Printf("humsafe server listening on %s", s.config.Addr)

	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		return s.server.ListenAndServeTLS(s.config.TLSCert, s.config.TLSKey)
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CORS — validate origin against allowed list
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range s.config.AllowedOrigins {
			if o == origin {
				allowed = true
				break
			}
		}
		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// CSP
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")

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

func authMiddleware(jwtSecret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := auth.ValidateJWT(tokenStr, jwtSecret)
		if err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyUserEmail, claims.Email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey string

const contextKeyUserEmail = contextKey("userEmail")

func (s *Server) registerRoutes() {
	// Health check (public)
	s.router.HandleFunc("GET /api/health", s.handleHealth)

	// Auth (public)
	s.router.HandleFunc("POST /api/auth/register", s.handleRegister)
	s.router.HandleFunc("POST /api/auth/login", s.handleLogin)

	// Public share retrieval
	s.router.HandleFunc("GET /api/share/{token}", s.handleGetShare)

	// Protected routes — require JWT auth
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/auth/totp/setup", s.handleTOTPSetup)
	protected.HandleFunc("POST /api/auth/totp/verify", s.handleTOTPVerify)

	protected.HandleFunc("GET /api/secrets/{env}", s.handleListSecrets)
	protected.HandleFunc("GET /api/secrets/{env}/{key}", s.handleGetSecret)
	protected.HandleFunc("PUT /api/secrets/{env}/{key}", s.handleSetSecret)
	protected.HandleFunc("DELETE /api/secrets/{env}/{key}", s.handleDeleteSecret)

	protected.HandleFunc("GET /api/environments", s.handleListEnvironments)
	protected.HandleFunc("POST /api/environments", s.handleCreateEnvironment)

	protected.HandleFunc("GET /api/users", s.handleListUsers)
	protected.HandleFunc("PUT /api/users/{id}/role", s.handleSetUserRole)

	protected.HandleFunc("GET /api/audit", s.handleAuditLog)

	protected.HandleFunc("POST /api/share", s.handleCreateShare)

	s.router.Handle("/", authMiddleware(s.config.JWTSecret, protected))
}

// JSON helper
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(v)
}
