package server

import (
	"net/http"
)

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// handleRegister creates a new user account.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleLogin authenticates a user and returns a JWT.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleTOTPSetup generates a TOTP secret for 2FA setup.
func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleTOTPVerify verifies a TOTP code.
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleListSecrets returns all secret keys for an environment.
func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleGetSecret returns a specific secret value.
func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleSetSecret creates or updates a secret.
func (s *Server) handleSetSecret(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleDeleteSecret removes a secret.
func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleListEnvironments returns all environments for the tenant.
func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleCreateEnvironment creates a new environment.
func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleListUsers returns all users for the tenant (admin only).
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleSetUserRole updates a user's role (admin only).
func (s *Server) handleSetUserRole(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleAuditLog returns the audit log for the tenant.
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleCreateShare creates a one-time share link.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// handleGetShare retrieves and consumes a one-time share.
func (s *Server) handleGetShare(w http.ResponseWriter, r *http.Request) {
	jsonError(w, "not implemented", http.StatusNotImplemented)
}
