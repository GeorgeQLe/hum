package db

// DB provides database access for the envsafe server.
// Currently a placeholder — full implementation requires a Postgres driver
// (github.com/jackc/pgx or github.com/lib/pq).
//
// All queries include tenant_id for row-level multi-tenancy isolation.

// Store defines the database operations for the envsafe server.
type Store interface {
	// Users
	CreateUser(tenantID, email, passwordHash, role string) (string, error)
	GetUserByEmail(tenantID, email string) (*User, error)
	SetUserRole(tenantID, userID, role string) error
	ListUsers(tenantID string) ([]User, error)

	// Environments
	CreateEnvironment(tenantID, name string) (string, error)
	ListEnvironments(tenantID string) ([]Environment, error)

	// Secrets
	SetSecret(tenantID, envID, key, encryptedValue string) error
	GetSecret(tenantID, envID, key string) (*Secret, error)
	ListSecrets(tenantID, envID string) ([]string, error)
	DeleteSecret(tenantID, envID, key string) error

	// Audit
	LogAction(tenantID, userID, action, resource, details, ip string) error
	GetAuditLog(tenantID string, limit int) ([]AuditEntry, error)

	// Share
	CreateShareLink(tenantID, token, encryptedValue, createdBy string, expiresIn int) error
	ConsumeShareLink(token string) (*ShareLink, error)
}

// User represents a database user record.
type User struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Role         string
	TOTPSecret   string
	TOTPEnabled  bool
}

// Environment represents a database environment record.
type Environment struct {
	ID       string
	TenantID string
	Name     string
}

// Secret represents a database secret record.
type Secret struct {
	ID             string
	TenantID       string
	EnvironmentID  string
	Key            string
	EncryptedValue string
	Version        int
}

// AuditEntry represents a database audit log record.
type AuditEntry struct {
	ID        string
	TenantID  string
	UserID    string
	Action    string
	Resource  string
	Details   string
	IPAddress string
	CreatedAt string
}

// ShareLink represents a database share link record.
type ShareLink struct {
	ID             string
	TenantID       string
	Token          string
	EncryptedValue string
	CreatedBy      string
	ExpiresAt      string
	ConsumedAt     string
}
