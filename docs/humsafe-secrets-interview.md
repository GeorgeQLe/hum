# humsafe — Secrets Interview Log

## Interview Context

Design interview for the humsafe encrypted environment variable manager, a standalone tool with first-class humrun integration.

---

## Q&A Decisions

### Encryption & Storage

**Q: What encryption algorithm for secrets at rest?**
A: AES-256-GCM (authenticated encryption). Master password derived via Argon2id key derivation function.

**Q: Where are vault files stored?**
A: `.humsafe/` directory in the project root. Contains:
- `vault.enc` — encrypted secrets (binary, AES-256-GCM)
- `config.json` — unencrypted metadata (project name, envs, team keys)
- `audit.log` — append-only signed audit trail

**Q: Is the vault safe to commit to git?**
A: Yes. The encrypted vault file is designed to be git-committed. Only `vault.enc` contains secret data and it's encrypted.

**Q: How is the encryption key cached?**
A: OS keychain integration via `go-keyring`:
- macOS: Keychain
- Linux: Secret Service (GNOME Keyring / KDE Wallet)
- Windows: Credential Manager

**Q: What about key recovery?**
A: Zero-knowledge for solo users — no recovery possible without the master password. Team/enterprise admins can re-encrypt vault keys for users.

### Secret Organization

**Q: How are secrets organized?**
A: Three-level hierarchy: Project > Environment > Key.
Example paths: `myapp/development/DB_URL`, `myapp/production/STRIPE_KEY`.

**Q: What's the default environment?**
A: `development`. Users can create arbitrary environments.

### CLI Design

**Q: What CLI framework?**
A: Cobra (matching humrun's existing pattern).

**Q: What commands are needed?**
A: See specification. Core: init, set, get, list, rm, env, unlock, lock, rotate. Team: user add/list/remove. Enterprise: audit, user role. Server: serve, login, share.

### Team Sharing

**Q: How do teams share secrets?**
A: Two mechanisms:
1. Git-committed vault with envelope encryption (X25519 key pairs per user)
2. Central server with REST API for enterprise

**Q: What's envelope encryption?**
A: The vault's symmetric key is encrypted separately for each team member using their public key. Each member can decrypt the vault key with their private key.

### Server

**Q: What server stack?**
A: Go HTTP server + Postgres. REST API. Row-level multi-tenancy.

**Q: Authentication?**
A: Email/password + TOTP 2FA + OIDC SSO.

**Q: RBAC model?**
A: Three roles — Admin (full access), Developer (read/write assigned envs), Viewer (read-only assigned envs).

### humrun Integration

**Q: How does humrun discover the vault?**
A: Checks for `.humsafe/` directory in the project root. If found and app has `vault_env` field, injects decrypted secrets into the process environment.

**Q: How are plain-text env vars migrated?**
A: `humsafe init` detects existing `env` values in apps.json and offers to import them into the vault, removing plain-text values.

**Q: Is env var exposure in process environment a concern?**
A: Yes, documented as a known limitation. Secrets are visible in `/proc/<pid>/environ` on Linux. Future: consider a secrets socket approach.

### Enterprise Features

**Q: What compliance reports?**
A: Access log, secret age, user permissions. Export as JSON, CSV, or plain text.

**Q: Secret rotation?**
A: Manual rotation with reminders. Previous values stored for rollback. Rotation history tracked per secret.

### UI

**Q: Web UI scope?**
A: Full secret management interface (not just a dashboard). Set/get/list secrets, manage environments, admin features.

**Q: TUI scope?**
A: Standalone Bubble Tea TUI via `humsafe browse`. Browse secrets, environments, users, audit logs.

---

## Key Deviations from Initial Discussion

1. **No automatic rotation** — manual rotation only, with reminder support
2. **No secrets socket** — direct env injection (documented limitation)
3. **Plain text export** — Go-native, no external dependencies
4. **REST over gRPC** — simpler for web UI consumption and broader compatibility
5. **Row-level multi-tenancy** — single database, `tenant_id` on every table (vs. schema-per-tenant)
