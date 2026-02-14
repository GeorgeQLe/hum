package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/georgele/devctl/internal/server/auth"
)

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// handleRegister creates a new user account.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || !strings.Contains(req.Email, "@") {
		jsonError(w, "invalid email", http.StatusBadRequest)
		return
	}
	if len(req.Password) < 8 {
		jsonError(w, "password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	// TODO: Store user in database
	_ = hash
	jsonOK(w, map[string]string{"status": "registered", "email": req.Email})
}

// handleLogin authenticates a user and returns a JWT.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Look up user in database and verify password with auth.VerifyPassword
	token, err := auth.GenerateJWT(req.Email, s.config.JWTSecret)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"token": token})
}

// handleTOTPSetup generates a TOTP secret for 2FA setup.
func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	email, _ := r.Context().Value(contextKeyUserEmail).(string)

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	uri := auth.TOTPProvisioningURI(secret, email, "envsafe")

	// TODO: Store secret in database associated with user
	jsonOK(w, map[string]string{"secret": secret, "provisioning_uri": uri})
}

// handleTOTPVerify verifies a TOTP code.
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Look up user's TOTP secret from database
	secret := "" // will come from DB lookup
	if secret == "" {
		jsonError(w, "TOTP not configured for user", http.StatusBadRequest)
		return
	}

	valid, err := auth.ValidateTOTPCode(secret, req.Code)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !valid {
		jsonError(w, "invalid TOTP code", http.StatusUnauthorized)
		return
	}

	jsonOK(w, map[string]string{"status": "verified"})
}

// handleListSecrets returns all secret keys for an environment.
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	env := r.PathValue("env")
	// TODO: Fetch from database with tenant isolation
	jsonOK(w, map[string]interface{}{"environment": env, "keys": []string{}})
}

// handleGetSecret returns a specific secret value.
func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	env := r.PathValue("env")
	key := r.PathValue("key")
	// TODO: Fetch from database with RBAC check
	jsonOK(w, map[string]string{"environment": env, "key": key, "value": ""})
}

// handleSetSecret creates or updates a secret.
func (s *Server) handleSetSecret(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	env := r.PathValue("env")
	key := r.PathValue("key")

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Store in database with audit log
	jsonOK(w, map[string]string{"environment": env, "key": key, "message": "secret set"})
}

// handleDeleteSecret removes a secret.
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	env := r.PathValue("env")
	key := r.PathValue("key")
	// TODO: Delete from database with audit log
	jsonOK(w, map[string]string{"environment": env, "key": key, "message": "secret deleted"})
}

// handleListEnvironments returns all environments for the tenant.
func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from database
	jsonOK(w, map[string]interface{}{"environments": []string{"development", "staging", "production"}})
}

// handleCreateEnvironment creates a new environment.
func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Create in database
	jsonOK(w, map[string]string{"name": req.Name, "message": "environment created"})
}

// handleListUsers returns all users for the tenant (admin only).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from database with admin check
	jsonOK(w, map[string]interface{}{"users": []string{}})
}

// handleSetUserRole updates a user's role (admin only).
func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	userID := r.PathValue("id")

	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role != "admin" && req.Role != "developer" && req.Role != "viewer" {
		jsonError(w, "invalid role", http.StatusBadRequest)
		return
	}

	// TODO: Update in database
	jsonOK(w, map[string]string{"user_id": userID, "role": req.Role, "message": "role updated"})
}

// handleAuditLog returns the audit log for the tenant.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from database with admin check
	jsonOK(w, map[string]interface{}{"entries": []string{}})
}

// handleCreateShare creates a one-time share link.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req struct {
		Environment string `json:"environment"`
		Key         string `json:"key"`
		ExpiresIn   int    `json:"expires_in"` // seconds
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Create encrypted share with expiry
	jsonOK(w, map[string]interface{}{"token": "share-token-placeholder", "expires_in": 3600, "message": "share link created"})
}

// handleGetShare retrieves and consumes a one-time share.
func (s *Server) handleGetShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	// TODO: Look up share, verify not expired, return value, mark as consumed
	jsonOK(w, map[string]string{"token": token, "message": "share consumed"})
}
