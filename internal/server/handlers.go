package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, `{"status":"ok"}`)
}

// handleRegister creates a new user account.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		jsonError(w, "email and password required", http.StatusBadRequest)
		return
	}

	// TODO: Store user in database
	jsonOK(w, fmt.Sprintf(`{"email":%q,"message":"user created"}`, req.Email))
}

// handleLogin authenticates a user and returns a JWT.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Verify credentials against database
	jsonOK(w, `{"token":"jwt-placeholder","message":"login successful"}`)
}

// handleTOTPSetup generates a TOTP secret for 2FA setup.
func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	// TODO: Generate TOTP secret and return QR code URL
	jsonOK(w, `{"message":"TOTP setup placeholder"}`)
}

// handleTOTPVerify verifies a TOTP code.
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	// TODO: Verify TOTP code
	jsonOK(w, `{"message":"TOTP verification placeholder"}`)
}

// handleListSecrets returns all secret keys for an environment.
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	env := r.PathValue("env")
	// TODO: Fetch from database with tenant isolation
	jsonOK(w, fmt.Sprintf(`{"environment":%q,"keys":[]}`, env))
}

// handleGetSecret returns a specific secret value.
func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	env := r.PathValue("env")
	key := r.PathValue("key")
	// TODO: Fetch from database with RBAC check
	jsonOK(w, fmt.Sprintf(`{"environment":%q,"key":%q,"value":""}`, env, key))
}

// handleSetSecret creates or updates a secret.
func (s *Server) handleSetSecret(w http.ResponseWriter, r *http.Request) {
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
	jsonOK(w, fmt.Sprintf(`{"environment":%q,"key":%q,"message":"secret set"}`, env, key))
}

// handleDeleteSecret removes a secret.
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	env := r.PathValue("env")
	key := r.PathValue("key")
	// TODO: Delete from database with audit log
	jsonOK(w, fmt.Sprintf(`{"environment":%q,"key":%q,"message":"secret deleted"}`, env, key))
}

// handleListEnvironments returns all environments for the tenant.
func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from database
	jsonOK(w, `{"environments":["development","staging","production"]}`)
}

// handleCreateEnvironment creates a new environment.
func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Create in database
	jsonOK(w, fmt.Sprintf(`{"name":%q,"message":"environment created"}`, req.Name))
}

// handleListUsers returns all users for the tenant (admin only).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from database with admin check
	jsonOK(w, `{"users":[]}`)
}

// handleSetUserRole updates a user's role (admin only).
func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
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
	jsonOK(w, fmt.Sprintf(`{"user_id":%q,"role":%q,"message":"role updated"}`, userID, req.Role))
}

// handleAuditLog returns the audit log for the tenant.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	// TODO: Fetch from database with admin check
	jsonOK(w, `{"entries":[]}`)
}

// handleCreateShare creates a one-time share link.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
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
	jsonOK(w, `{"token":"share-token-placeholder","expires_in":3600,"message":"share link created"}`)
}

// handleGetShare retrieves and consumes a one-time share.
func (s *Server) handleGetShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	// TODO: Look up share, verify not expired, return value, mark as consumed
	jsonOK(w, fmt.Sprintf(`{"token":%q,"message":"share consumed"}`, token))
}
