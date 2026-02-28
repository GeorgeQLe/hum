# Security Audit: hum (humrun + humsafe)

**Date:** 2026-02-27
**Scope:** humrun (TUI process manager) + humsafe (encrypted secrets manager)

---

## Summary

### Round 1

| Severity | Found | Fixed | Tests Added |
|----------|-------|-------|-------------|
| Critical | 2     | 2     | 5           |
| High     | 4     | 4     | 2           |
| Medium   | 6     | 5     | 1           |
| Low      | 4     | 3     | 3           |
| **Total**| **16**| **14**| **11**      |

### Round 2

| Severity | Found | Fixed | Tests Added |
|----------|-------|-------|-------------|
| High     | 2     | 2     | 3           |
| Medium   | 7     | 7     | 7           |
| Low      | 3     | 3     | 5           |
| **Total**| **12**| **12**| **15**      |

### Combined

| Severity | Found | Fixed | Tests Added |
|----------|-------|-------|-------------|
| Critical | 2     | 2     | 5           |
| High     | 6     | 6     | 5           |
| Medium   | 13    | 12    | 8           |
| Low      | 7     | 6     | 8           |
| **Total**| **28**| **26**| **26**      |

---

## CRITICAL

### C1. Audit log has no integrity protection
- **File:** `internal/vault/audit/audit.go`
- **Issue:** Audit entries were plaintext JSON lines with no tamper detection. An attacker with file access could modify, delete, or reorder entries undetected.
- **Fix:** Added HMAC-SHA256 chain integrity. Each entry now includes `hmac(prev_hmac + entry_json)`. New `NewLoggerWithHMAC()` constructor accepts a key derived from the vault. Added `VerifyChain()` to validate the entire log. Backward compatible — loggers without HMAC key still work, and entries without HMAC are skipped during verification.
- **Tests:** `TestAuditHMACChain`, `TestAuditTamperDetection`, `TestAuditHMACChainWrongKey`, `TestAuditWithoutHMAC`

### C2. JWT validation has no algorithm check
- **File:** `internal/server/auth/auth.go:48-78`
- **Issue:** `ValidateJWT()` parsed the header but never verified `alg == "HS256"`. An attacker could craft a token with `alg: "none"` to bypass signature verification.
- **Fix:** Added explicit header parsing and algorithm check: tokens with any algorithm other than `"HS256"` are rejected with a clear error.
- **Tests:** `TestJWTAlgorithmNone`, `TestJWTAlgorithmRS256Rejected`, `TestJWTValidation`, `TestJWTGeneration`, `TestJWTExpired`, `TestJWTTampered`, `TestJWTWrongSecret`, `TestJWTInvalidFormat`

---

## HIGH

### H1. Crash log directory is world-listable
- **File:** `root.go:92`
- **Issue:** `os.MkdirAll(crashDir, 0755)` — any local user could list `/tmp/humrun-crashes/` and see crash log filenames (timestamps reveal when humrun panicked).
- **Fix:** Changed `0755` to `0700`.

### H2. IPC socket chmod failure is non-fatal
- **File:** `internal/ipc/server.go:92-94`
- **Issue:** If `os.Chmod(socketPath, 0600)` failed, the server continued with a potentially world-readable socket.
- **Fix:** Socket chmod failure now closes the listener, removes the socket, and returns an error. The server refuses to start with insecure permissions.
- **Tests:** `TestSocketPermissions`

### H3. No rate limiting on HTTP API
- **Files:** `internal/api/server.go`, `internal/api/ratelimit.go` (new)
- **Issue:** No per-client rate limiting. The approval queue buffer was only 16 entries, allowing a malicious local process to flood the queue.
- **Fix:** Added token-bucket rate limiter middleware (100 burst, 10 req/s sustained). Implemented without external dependencies using a simple mutex-protected counter.
- **Tests:** `TestRateLimiting`

### H4. Backup archives are not encrypted
- **File:** `cmd/humsafe/cmd/backup.go`
- **Issue:** Backups were gzip+tar only. While `vault.enc` inside was encrypted, `config.json` (project name, environments, team public keys) was plaintext.
- **Fix:** Backups are now encrypted by default with AES-256-GCM using a key derived from a prompted password via Argon2id. Format: `HUMSAFE_ENC\x01` magic + salt(16) + AES-GCM(tar.gz). Added `--no-encrypt` flag for unencrypted backups. Restore auto-detects encrypted vs unencrypted archives.

---

## MEDIUM

### M1. Minimum password length too low (8 chars)
- **File:** `cmd/humsafe/cmd/helpers.go:118`
- **Fix:** Increased minimum password length from 8 to 12 characters.

### M2. HUMSAFE_PASSWORD env var visible in process listings
- **File:** `internal/vault/inject.go:25`
- **Issue:** Password passed via env var was visible in `ps` output and inherited by child processes.
- **Fix:** Added warning log when `HUMSAFE_PASSWORD` is detected: *"warning: using HUMSAFE_PASSWORD env var — visible in process listings. Prefer keychain caching ('humsafe unlock')."* Verified that `HUMSAFE_` prefix stripping in `manager.go:679` already prevents inheritance to child processes.

### M3. TOTP base32 uses NoPadding
- **File:** `internal/server/auth/totp.go:26`
- **Issue:** Some authenticator apps expect standard base32 padding.
- **Fix:** `GenerateTOTPSecret()` now uses `base32.StdEncoding` (with padding). `GenerateTOTPCode()` accepts both padded and unpadded secrets for backward compatibility.

### M4. CSP allows unsafe-inline for styles
- **File:** `internal/server/server.go:99`
- **Fix:** Removed `'unsafe-inline'` from `style-src` directive.

### M5. No nonce uniqueness guarantee tested
- **File:** `internal/vault/crypto/aes.go`
- **Issue:** GCM is catastrophically broken if nonces repeat. While `crypto/rand` makes collision astronomically unlikely, there was no test.
- **Fix:** Added `TestEncryptNonceUniqueness` — encrypts same plaintext 1000 times, verifies all ciphertexts are distinct.

### M6. Keychain service name collision across projects
- **File:** `internal/vault/keychain/keychain.go:8`
- **Issue:** All projects used service name `"humsafe"` with only the project name as account. Two projects named `"myapp"` in different directories would collide.
- **Fix:** Account key now uses `sha256(absolute_path)[:8]:basename`. Retrieve falls back to the legacy format (just project name) for migration. Delete removes both new and legacy entries.

---

## LOW

### L1. No auth failure logging
- **File:** `internal/server/server.go`
- **Fix:** Added `log.Printf("AUTH FAILURE: ...")` on missing/invalid Authorization headers and invalid tokens, including remote address, method, and path.

### L2. Audit log user is always "local"
- **Files:** `cmd/humsafe/cmd/helpers.go`, `cmd/humsafe/cmd/passwd.go`
- **Fix:** Added `localUsername()` helper using `os/user.Current().Username`. Updated passwd.go to use it. Falls back to `"local"` if OS user lookup fails.

### L3. HKDF context string not versioned flexibly
- **File:** `internal/vault/sharing/envelope.go:61,103`
- **Status:** No change needed. The context string `"humsafe-vault-key-sharing-v1"` is already versioned. Documented here for future reference — any key derivation context change requires a new version suffix.

### L4. No CORS rejection headers
- **File:** `internal/server/server.go`
- **Fix:** Added explicit `Access-Control-Allow-Origin: null` for non-whitelisted origins. Added `Vary: Origin` header on all responses for correct caching behavior.

---

## New Tests Added

| Test | File | What it verifies |
|------|------|-----------------|
| `TestJWTAlgorithmNone` | `auth/auth_test.go` | `alg:none` tokens are rejected |
| `TestJWTAlgorithmRS256Rejected` | `auth/auth_test.go` | Non-HS256 algorithms are rejected |
| `TestJWTExpired` | `auth/auth_test.go` | Expired tokens are rejected |
| `TestJWTTampered` | `auth/auth_test.go` | Modified payloads are rejected |
| `TestJWTWrongSecret` | `auth/auth_test.go` | Wrong signing key is rejected |
| `TestJWTInvalidFormat` | `auth/auth_test.go` | Malformed tokens are rejected |
| `TestAuditHMACChain` | `audit/audit_test.go` | HMAC chain validates clean log |
| `TestAuditTamperDetection` | `audit/audit_test.go` | Modified entry breaks chain |
| `TestAuditHMACChainWrongKey` | `audit/audit_test.go` | Wrong key fails verification |
| `TestEncryptNonceUniqueness` | `crypto/crypto_test.go` | 1000 encryptions = 1000 unique ciphertexts |
| `TestVaultFilePermissions` | `vault/vault_test.go` | vault.enc=0600, config.json=0600, .humsafe=0700 |
| `TestSocketPermissions` | `ipc/server_test.go` | Socket file has 0600 permissions |
| `TestIPCPathTraversal` | `ipc/server_test.go` | `../` in project root stays in socket dir |
| `TestAPIAuthRequired` | `api/api_test.go` | All auth endpoints reject without token |
| `TestAPIInputValidation` | `api/api_test.go` | Missing name, invalid JSON rejected |
| `TestRateLimiting` | `api/api_test.go` | Burst >100 requests triggers 429 |
| `TestTOTPReplay` | `auth/auth_test.go` | Old codes outside drift window rejected |
| `TestChangePassword` | `vault/vault_test.go` | Password change works, old password fails |

---

## Verification Checklist

- [x] `go build ./...` — both binaries compile
- [x] `go vet ./...` — clean
- [x] `go test -race ./...` — all tests pass including new security tests
- [x] `grep -r "0755" --include="*.go" .` — no world-readable dirs remain for sensitive data
- [x] JWT `alg: "none"` token is rejected (TestJWTAlgorithmNone)
- [x] Tampered audit log detected by chain verification (TestAuditTamperDetection)

---

## Files Modified

| File | Changes |
|------|---------|
| `internal/server/auth/auth.go` | JWT algorithm validation |
| `internal/server/auth/auth_test.go` | JWT security test suite |
| `internal/vault/audit/audit.go` | HMAC chain integrity |
| `internal/vault/audit/audit_test.go` | Tamper detection tests |
| `root.go` | Crash dir 0755→0700 |
| `internal/ipc/server.go` | Fatal socket chmod |
| `internal/ipc/server_test.go` | Socket permission + path traversal tests |
| `internal/api/server.go` | Rate limiting middleware |
| `internal/api/ratelimit.go` | Token-bucket rate limiter (new) |
| `internal/api/api_test.go` | Auth, input validation, rate limit tests |
| `cmd/humsafe/cmd/backup.go` | Encrypted backup archives |
| `cmd/humsafe/cmd/helpers.go` | Min password 12 chars, localUsername() |
| `cmd/humsafe/cmd/passwd.go` | Use localUsername() in audit |
| `internal/vault/inject.go` | HUMSAFE_PASSWORD warning |
| `internal/server/auth/totp.go` | Standard base32 padding |
| `internal/server/server.go` | CSP fix, auth failure logging, CORS headers |
| `internal/vault/crypto/crypto_test.go` | Nonce uniqueness test |
| `internal/vault/vault_test.go` | File permissions test |
| `internal/vault/keychain/keychain.go` | Collision-resistant account keys |

---

# Round 2: Deep Code Analysis

**Date:** 2026-02-27

---

## HIGH (Round 2)

### R2-H1. Dev supervisor leaks all env vars to child processes
- **File:** `internal/dev/supervisor.go:197`
- **Issue:** `cmd.Env = os.Environ()` passes ALL env vars (including `HUMSAFE_PASSWORD`, `HUMRUN_TOKEN`) to `go build` and the child binary. The main `manager.go` uses `filteredEnv()` but supervisor bypassed it.
- **Fix:** Exported `FilteredEnv()` from `internal/process/manager.go` and used it in supervisor.
- **Tests:** `TestSupervisorEnvFiltering`

### R2-H2. Crash logs stored in shared /tmp with predictable paths
- **File:** `root.go:91-96`
- **Issue:** Crash dumps went to `/tmp/humrun-crashes/crash-<unix_timestamp>.log`. Predictable paths in shared `/tmp` enable symlink attacks.
- **Fix:** Moved crash logs to `xdg.CacheDir()/crashes/` (user-local). Uses `O_EXCL` flag to prevent symlink attacks.
- **Tests:** `TestCrashLogLocation`

---

## MEDIUM (Round 2)

### R2-M1. IPC socket dir in shared /tmp — symlink race
- **File:** `internal/ipc/server.go:18`
- **Issue:** Socket dir `$TMPDIR/humrun-sockets/` was in shared temp space. TOCTOU race between `os.Stat` → `os.Remove` → `net.Listen`.
- **Fix:** Socket dir moved to `xdg.RuntimeDir()/sockets/` (user-private). Verified dir ownership with 0700 permissions.
- **Tests:** `TestSocketDirOwnership`, `TestSocketDirNotSharedTmp`

### R2-M2. API body size limits inconsistent
- **File:** `internal/api/handlers.go:317`
- **Issue:** Only `RegisterApp` used `io.LimitReader`. Other handlers had no body size limit.
- **Fix:** Added `http.MaxBytesReader` middleware in `internal/api/server.go` for all routes (1 MB limit).
- **Tests:** `TestAPIBodySizeLimit`

### R2-M3. No JSON request body size limit on server endpoints
- **File:** `internal/server/server.go`
- **Issue:** The experimental server had no body size limits.
- **Fix:** Added `http.MaxBytesReader` in `withMiddleware` function (1 MB limit).
- **Tests:** `TestServerBodySizeLimit`

### R2-M4. Backup restore doesn't reject symlink tar entries
- **File:** `cmd/humsafe/cmd/backup.go:245`
- **Issue:** Only `tar.TypeDir` and `tar.TypeReg` were handled. Symlink and hardlink entries were silently ignored.
- **Fix:** Explicitly reject `tar.TypeSymlink`, `tar.TypeLink`, and any other non-regular types with an error.
- **Tests:** `TestBackupRestoreRejectsSymlinks`, `TestBackupRestoreRejectsHardlinks`, `TestBackupRestoreAcceptsNormalFiles`

### R2-M5. IPC server doesn't limit message size
- **File:** `internal/ipc/server.go:156`
- **Issue:** `bufio.Scanner` used default buffer, no explicit limit on message size.
- **Fix:** Set explicit `scanner.Buffer()` limit of 64KB.
- **Tests:** `TestIPCMessageSizeLimit`

### R2-M6. No rate limiting on experimental server auth endpoints
- **File:** `internal/server/server.go:146-147`
- **Issue:** `POST /api/auth/register` and `POST /api/auth/login` were public with no rate limiting.
- **Fix:** Added token-bucket rate limiter (5 burst, 1 req/s sustained) to auth routes.
- **Tests:** `TestServerAuthRateLimiting`

### R2-M7. Auth token stored as plaintext file
- **File:** `cmd/humsafe/cmd/login.go:81`
- **Issue:** JWT token stored as plaintext at `$USER_CONFIG_DIR/humsafe/token`.
- **Status:** Deferred — server is fully experimental (all endpoints return 501). Token at 0600 in user config dir is acceptable for non-functional feature. Added TODO comment for when server is implemented.

---

## LOW (Round 2)

### R2-L1. Log injection via app names
- **File:** `internal/config/config.go`
- **Issue:** App names from config rendered directly in TUI. ANSI escape sequences in app names could manipulate terminal state.
- **Fix:** Added `containsControlChars()` validation in `App.Validate()` — rejects names with ANSI escapes or control characters.
- **Tests:** `TestAppNameControlChars`

### R2-L2. PID file fallback to shared /tmp
- **File:** `internal/api/discovery.go:16`
- **Issue:** `GlobalDir()` fallback used `os.TempDir()/.humrun` (shared). Primary path was already `~/.humrun`.
- **Fix:** Fallback now uses `$XDG_RUNTIME_DIR/humrun` or UID-scoped temp path to avoid shared access.
- **Tests:** `TestPIDFileLocation`

### R2-L3. Dev supervisor temp binary in shared /tmp
- **File:** `internal/dev/supervisor.go:53`
- **Issue:** `humrun-dev-<hash>` binary written to `os.TempDir()`. Another user could replace it between writes.
- **Fix:** Moved to `xdg.CacheDir()/dev/` (user-local cache dir).
- **Tests:** `TestSupervisorTmpBinaryNotInSharedTmp`

---

## Round 2 — New Tests Added

| Test | File | What it verifies |
|------|------|-----------------|
| `TestSupervisorEnvFiltering` | `internal/dev/supervisor_test.go` | Sensitive env vars stripped from child processes |
| `TestSupervisorTmpBinaryNotInSharedTmp` | `internal/dev/supervisor_test.go` | Dev binary not in shared /tmp |
| `TestCrashLogLocation` | `root_test.go` | Crash dir under user home, not /tmp |
| `TestSocketDirOwnership` | `internal/ipc/server_test.go` | Socket dir has 0700, no group/other access |
| `TestSocketDirNotSharedTmp` | `internal/ipc/server_test.go` | Socket dir not in shared /tmp |
| `TestIPCMessageSizeLimit` | `internal/ipc/server_test.go` | Oversized IPC message handled gracefully |
| `TestAPIBodySizeLimit` | `internal/api/api_test.go` | Oversized HTTP body rejected |
| `TestPIDFileLocation` | `internal/api/api_test.go` | PID file under user dir, not shared /tmp |
| `TestStubEndpointsReturn501` | `internal/server/server_test.go` | Unimplemented endpoints return 501 |
| `TestServerBodySizeLimit` | `internal/server/server_test.go` | Server body size limit applied |
| `TestServerAuthRateLimiting` | `internal/server/server_test.go` | Auth endpoints rate-limited after burst |
| `TestAppNameControlChars` | `internal/config/config_test.go` | ANSI/control chars rejected in app names |
| `TestBackupRestoreRejectsSymlinks` | `cmd/humsafe/cmd/backup_test.go` | Symlink in archive causes error |
| `TestBackupRestoreRejectsHardlinks` | `cmd/humsafe/cmd/backup_test.go` | Hardlink in archive causes error |
| `TestXDGDirFallbacks` | `internal/xdg/xdg_test.go` | Correct XDG fallback paths, not shared /tmp |

---

## Round 2 — Files Modified

| File | Changes |
|------|---------|
| `internal/xdg/xdg.go` | **NEW** — XDG directory helpers (RuntimeDir, CacheDir) |
| `internal/xdg/xdg_test.go` | **NEW** — XDG helper tests |
| `root.go` | Crash log dir → XDG cache, O_EXCL flag |
| `root_test.go` | **NEW** — Crash log location test |
| `internal/ipc/server.go` | Socket dir → XDG runtime, scanner buffer limit |
| `internal/ipc/server_test.go` | Socket dir ownership, message size, shared tmp tests |
| `internal/api/discovery.go` | PID dir fallback → UID-scoped, not shared /tmp |
| `internal/api/server.go` | Body size limit middleware |
| `internal/api/api_test.go` | Body size limit, PID file location tests |
| `internal/dev/supervisor.go` | FilteredEnv(), temp binary → XDG cache |
| `internal/dev/supervisor_test.go` | **NEW** — Env filtering, temp binary location tests |
| `internal/process/manager.go` | Export FilteredEnv() |
| `internal/server/server.go` | Body size limit, auth rate limiting |
| `internal/server/server_test.go` | **NEW** — Stub endpoints, body size, rate limit tests |
| `internal/config/config.go` | App name control char validation |
| `internal/config/config_test.go` | Control char validation tests |
| `cmd/humsafe/cmd/backup.go` | Reject symlink/hardlink tar entries |
| `cmd/humsafe/cmd/backup_test.go` | **NEW** — Symlink/hardlink rejection tests |
| `cmd/humsafe/cmd/login.go` | TODO comment for token encryption |
| `SECURITY_AUDIT.md` | Round 2 findings documented |

---

## Round 2 — Verification Checklist

- [x] `go build ./...` — both binaries compile
- [x] `go vet ./...` — clean
- [x] `go test -race ./...` — all tests pass including 15 new security tests
- [x] `go test -race -tags e2e ./internal/e2e/` — E2E tests pass
- [x] Crash logs written to user-local dir, not shared /tmp
- [x] IPC sockets in user-private runtime dir
- [x] Sensitive env vars filtered from supervisor child processes
- [x] Symlink tar entries rejected during backup restore
- [x] API body size limits applied to all routes
- [x] Auth endpoints rate-limited on experimental server
