-- humsafe initial database schema
-- Row-level multi-tenancy: tenant_id on every table

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    plan        TEXT NOT NULL DEFAULT 'free',  -- free, team, enterprise
    created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    email       TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT 'developer',  -- admin, developer, viewer
    totp_secret TEXT,
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, email)
);

CREATE TABLE IF NOT EXISTS environments (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    name        TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS secrets (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    environment_id  TEXT NOT NULL REFERENCES environments(id),
    key             TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, environment_id, key)
);

CREATE TABLE IF NOT EXISTS secret_history (
    id          TEXT PRIMARY KEY,
    secret_id   TEXT NOT NULL REFERENCES secrets(id),
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    encrypted_value TEXT NOT NULL,
    version     INTEGER NOT NULL,
    rotated_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_log (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    user_id     TEXT REFERENCES users(id),
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    details     TEXT,
    ip_address  TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS share_links (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id),
    token           TEXT NOT NULL UNIQUE,
    encrypted_value TEXT NOT NULL,
    created_by      TEXT NOT NULL REFERENCES users(id),
    expires_at      TIMESTAMP NOT NULL,
    consumed_at     TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_secrets_tenant_env ON secrets(tenant_id, environment_id);
CREATE INDEX IF NOT EXISTS idx_audit_tenant ON audit_log(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_share_token ON share_links(token);
