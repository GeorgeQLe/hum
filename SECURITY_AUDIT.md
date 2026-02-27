# Security Audit: hum (humrun + humsafe)

**Date:** 2026-02-27
**Scope:** humrun (TUI process manager) + humsafe (encrypted secrets manager)

---

## Summary

| Severity | Found | Fixed | Tests Added |
|----------|-------|-------|-------------|
| Critical | 2     | 2     | 5           |
| High     | 4     | 4     | 2           |
| Medium   | 6     | 5     | 1           |
| Low      | 4     | 3     | 3           |
| **Total**| **16**| **14**| **11**      |

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
