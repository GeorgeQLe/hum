# devctl - Audit PR List

50 PRs split into 10 phases. Each phase can be developed, tested, and merged independently.
Later phases build on earlier ones, so execute in order.

---

## Phase 1: Foundation (Split Monoliths + CI)

Everything else depends on this. Split the giant files so PRs don't conflict, add CI so every subsequent phase is validated automatically.

- [ ] **PR #1: Split `main.go` (21K lines) into proper Cobra command files** - Extract each subcommand (`ping`, `status`, `add`, `stats`, `scan`, `dev`, `start`, `stop`, `restart`) into `cmd/devctl/` files following standard Cobra patterns. Single biggest maintainability blocker.
- [ ] **PR #2: Split `tui/model.go` (3,444 lines) into composable sub-models** - Refactor into sub-models with their own `Update()` / `View()` methods. Reduces cognitive load, enables independent testing, prevents merge conflicts.
- [ ] **PR #3: Add CI pipeline (GitHub Actions)** - `go vet`, `staticcheck`, `go test -race ./...`, `golangci-lint`, build matrix (linux/darwin/windows), release automation via GoReleaser.
- [ ] **PR #4: Return 501 Not Implemented for stub endpoints** - `internal/server/handlers.go` has ~8 handler stubs (register, login, TOTP, secrets CRUD) returning 200 OK with placeholder bodies.
- [ ] **PR #48: Add golangci-lint config and fix all warnings** - Add `.golangci.yml` with reasonable defaults, fix existing warnings, add to CI.

**Test milestone:** `go build ./...` passes, CI green, all existing tests pass, linter clean.

---

## Phase 2: Correctness (Bug Fixes)

Fix known bugs while the codebase is freshly split. No new features -- just make existing behavior correct.

- [ ] **PR #5: Fix event channel overflow dropping critical state changes** - `process/manager.go:627-644` has a 1024-event buffer. When full, critical events (crash, restart) are silently dropped. Add priority queue and backpressure.
- [ ] **PR #7: Fix graceful shutdown ordering** - Session state saved *before* processes stopped (`model.go:2078`). Fix: stop processes -> confirm shutdown -> save final state. Increase 10s timeout.
- [ ] **PR #8: Fix panic recovery to restore terminal state** - `main.go:194-200` catches panics but `p.Kill()` doesn't guarantee terminal restoration. Add cleanup, remove IPC socket, write stack trace to file.
- [ ] **PR #19: Increase event buffer and add backpressure** - Increase to 8192, add ring-buffer semantics, implement backpressure via `select` with `default` to log drops. (Companion to #5.)
- [ ] **PR #21: Fix port scan range (hardcoded 9999 ceiling)** - `process/port.go:83` scans `basePort..9999`. Should scan to 65535 or configurable upper bound.
- [ ] **PR #46: Fix all silent error swallows** - 8+ locations where errors assigned to `_` in critical paths: `json.Marshal` in IPC (`ipc/server.go:150,162`), `os.WriteFile` (`config/config.go:232`), `syscall.Kill` (`manager.go:392,402`).
- [ ] **PR #47: Deduplicate supervisor event loops** - `dev/supervisor.go` has two nearly identical event loops (lines 96-164 and 322-380). Extract into shared `runEventLoop()`.

**Test milestone:** Start 5+ apps, kill -9 one, verify no dropped events. Graceful quit saves correct state. Panic in TUI restores terminal. Port auto-assign works above 9999.

---

## Phase 3: Observability (Logging & Debugging)

Add structured logging so all future development is easier to debug. Must land before major new features.

- [ ] **PR #26: Add structured logging with slog** - Replace mixed `log.Printf` / `fmt.Fprintf(os.Stderr)` / silent swallows with Go 1.21+ `log/slog`. Add levels, JSON output mode.
- [ ] **PR #27: Add `--debug` flag with verbose output** - Enable debug-level logging, print IPC messages, show config load steps, report event channel utilization, dump process manager state.
- [ ] **PR #28: Surface dropped events and error counts in TUI** - Add status bar indicator for dropped events, error buffer size, memory usage, IPC queue depth.
- [ ] **PR #29: Add `devctl debug dump` for bug reports** - Dump Go version, OS, terminal size, running apps, port allocations, IPC status, event queue depth, memory usage, last 100 log lines per app. Anonymize secrets.

**Test milestone:** `devctl dev --debug` produces structured JSON logs. `devctl debug dump` outputs valid diagnostic report. TUI status bar shows event/error counts.

---

## Phase 4: Testing Infrastructure

Now that the code is split, correct, and observable -- add proper test coverage to lock it in.

- [ ] **PR #30: Add integration test suite for TUI** - Use `teatest` for mode transitions, keyboard handling, layout at various terminal sizes, and approval flow.
- [x] **PR #31: Add end-to-end test with real processes** - 19 e2e tests in `internal/e2e/` behind build tag. Manager lifecycle (start, stop, crash, restart, env, error detection, process group kill), HTTP API (auth, start/stop, detail, logs, ports), IPC (ping, start/stop, status), health checks (healthy/unhealthy transitions). CI job and `make test-e2e` target.
- [ ] **PR #32: Add fuzz testing for config parser** - `go test -fuzz` for malformed JSON, extreme values, unicode app names, empty arrays, circular dependencies.
- [ ] **PR #33: Add benchmark tests for hot paths** - `Benchmark*` tests for log buffer append/trim, TUI view rendering, event channel throughput, ANSI sanitization.

**Test milestone:** `go test -race ./...` covers TUI modes, E2E lifecycle, config edge cases. Benchmarks establish performance baselines. CI runs all of the above.

---

## Phase 5: CLI & Developer Experience

Add new subcommands and quality-of-life improvements. Each is an independent feature that can be tested in isolation.

- [ ] **PR #11: Add `devctl init` interactive scaffolding** - Detect package managers, ask for app names/ports/commands, generate `apps.json`, optionally run `devctl scan`.
- [ ] **PR #12: Add `devctl validate` for config linting** - Validate JSON syntax, check fields against schema, detect dependency cycles, validate port ranges, warn about missing health checks.
- [ ] **PR #14: Add `devctl logs` CLI subcommand** - `devctl logs <app> [--follow] [--lines N] [--since 5m]` connecting via IPC, streaming to stdout.
- [ ] **PR #15: Support JSONC or YAML for apps.json** - JSON has no comments. Support JSONC or YAML as alternative formats with auto-detection.
- [ ] **PR #16: Add `devctl doctor` diagnostic command** - Verify `lsof` availability, check port conflicts, validate config, test IPC socket, verify dependencies, check disk space.
- [ ] **PR #17: Add shell completions (bash/zsh/fish)** - `devctl completion bash|zsh|fish` using Cobra's built-in generator. Include completions for app names.
- [ ] **PR #37: Add man pages and `--help` improvements** - Detailed `--help` with examples, `devctl help <topic>` for concepts, generate man pages from Cobra.

**Test milestone:** `devctl init` generates valid config. `devctl validate` catches bad configs. `devctl logs` streams in real time. `devctl doctor` reports all checks. Completions load in each shell.

---

## Phase 6: Log Pipeline (Persist + Stream + Export)

Currently logs are memory-only. This phase adds disk persistence, real-time HTTP streaming, and batch export.

- [ ] **PR #6: Persist logs to disk with rotation** - Add per-app log files under `.devctl/logs/`, configurable retention (lines/time/size), and `devctl logs --tail`.
- [ ] **PR #13: Add real-time log streaming to HTTP API** - `api/handlers.go:164` has a stub. Implement proper SSE push from log buffer so external tools can tail logs.
- [ ] **PR #44: Add log export (JSON, file, S3)** - `devctl logs export --format json|text --output ./logs/` for batch export, `--stream-to` for continuous export.

**Test milestone:** Kill TUI, restart, verify logs survived on disk. `curl` the SSE endpoint and see live log lines. Export produces valid JSON/text files. Rotation caps disk usage.

---

## Phase 7: Security Hardening

Harden IPC, HTTP API, vault, and command execution. Each PR is independently testable.

- [ ] **PR #10: Fix IPC socket security -- use XDG_RUNTIME_DIR** - Move socket to `$XDG_RUNTIME_DIR/devctl/` (Linux) or `~/Library/Caches/devctl/` (macOS) with 0700 permissions. Add client auth via nonce.
- [ ] **PR #22: Validate and sandbox shell commands from apps.json** - Add allowlist validation, `--dry-run` preview, and warnings for shell metacharacters (`;`, `&&`, `|`).
- [ ] **PR #23: Rotate API bearer tokens** - Add token expiry (24h), auto-rotation on restart, `devctl token rotate`, and token scoping (read-only vs mutating).
- [ ] **PR #24: Add rate limiting to IPC and HTTP API** - Per-client limits (100 req/s IPC, 60 req/min HTTP mutations) with 429 responses.
- [ ] **PR #25: Audit logging for all secret operations** - Add structured audit entries for access, modify, rotate, share, delete. Feed into `vault/audit/audit.go`.

**Test milestone:** Socket permissions are 0700. Stale tokens rejected. Rate limiter returns 429 under load. Audit log records all vault operations. `--dry-run` previews commands without executing.

---

## Phase 8: Health & Lifecycle

Wire health checks into process management, add lifecycle hooks, environment injection, and dependency gating.

- [ ] **PR #9: Integrate health checks with auto-restart logic** - Wire health status into supervisor: unhealthy for N checks -> restart. App can be "running" (PID alive) while returning 500s.
- [ ] **PR #38: Add `.env` file support per app** - Per-app `.env` paths in config, auto-reload on change, variable expansion, integration with envsafe vault injection.
- [ ] **PR #39: Add pre/post lifecycle hooks** - `preStart`, `postStart`, `preStop`, `postStop` hooks running shell commands. Use case: migrations before server start.
- [ ] **PR #41: Add `devctl exec <app> <command>`** - Run one-off command with app's env/cwd/port config. Essential for migrations, seeds, REPL.
- [ ] **PR #42: Add dependency health gating** - Wait for dependency health check to pass before starting dependent app. Configurable timeout and failure behavior.

**Test milestone:** App returning 500 gets auto-restarted after N failures. `.env` vars visible in `devctl exec`. Hooks fire in correct order. Dependent app waits for healthy upstream.

---

## Phase 9: Performance

Optimize the TUI now that all features are in. Benchmark tests from Phase 4 catch regressions.

- [ ] **PR #18: Cache TUI layout computation** - `model.go` calls `recalcLayout()` on every `View()`. Cache on `WindowSizeMsg` only. Memoize rendered sidebar/log content between frames.
- [ ] **PR #20: Lazy-load process logs in TUI** - Only render selected app's log pane. For sidebar apps, show last-line preview but skip full ANSI rendering until focused.

**Test milestone:** Benchmark tests show measurable improvement. TUI stays responsive with 50+ apps. No visual regressions in integration tests.

---

## Phase 10: Ship It (Distribution + Cleanup)

Package for distribution, add remaining features, and clean up dead code.

- [ ] **PR #34: Add Dockerfile and docker-compose example** - Multi-stage Dockerfile + `docker-compose.yml` showing devctl managing services in a container.
- [ ] **PR #35: Add Homebrew formula** - Homebrew tap with formula for `brew install devctl`. Pre-built bottles for common platforms.
- [ ] **PR #36: Add GoReleaser config for cross-platform releases** - `.goreleaser.yml` for linux/darwin/windows (amd64/arm64), GitHub releases with checksums, changelog.
- [ ] **PR #40: Add webhook/notification integration** - Slack webhook on crash, generic webhook POST on state change, `notify-send` on Linux, configurable rules per app.
- [ ] **PR #43: Add process resource limits** - Optional `resources: { maxMemoryMB: 512, cpuLimit: 50 }` in apps.json. Use `ulimit` (macOS) or cgroups (Linux).
- [ ] **PR #45: Add HTTP API approval endpoint** - `POST /api/approvals/{id}/approve` and `POST /api/approvals/{id}/deny` for CI/CD and external tools.
- [ ] **PR #49: Add Go doc comments to all exported symbols** - `godoc`-compatible comments to all public APIs across all packages.
- [ ] **PR #50: Remove dead code and TODO stubs** - Placeholder directories (`server/handlers/`, `envsafe-tui/views/`), incomplete DB queries. Implement or remove.

**Test milestone:** `goreleaser --snapshot` produces binaries for all platforms. `brew install` works. Docker image builds and runs. All TODOs resolved or removed.

---

## Previously Completed

- [x] **Config File Watcher** - Auto-reload `apps.json` when it changes on disk
- [x] **Log Timestamps** - Toggle (`t` key) to prefix log lines with timestamps
- [x] **Session Persistence** - Save running apps on exit to `.devctl-state.json`; `--restore` flag
- [x] **Quick Keyboard Shortcuts** - `s`/`S`/`r` shortcuts in sidebar for start/stop/restart
- [x] **Production Readiness Fixes** — Concurrency, security, and reliability hardening:
  - Fix data race on `m.apps` between HTTP goroutines and Bubble Tea (channel-based mutations + RWMutex snapshot)
  - Fix `ErrorBuffer.Errors` direct access without lock (added `SnapshotErrors()`)
  - Fix `Entry.Cmd = nil` mutation from wrong goroutine (added `GetDetail()`)
  - Constant-time bearer token comparison (`subtle.ConstantTimeCompare`)
  - Strip sensitive env vars (`ENVSAFE_`, `DEVCTL_TOKEN`) from child processes
  - Path traversal check in API register endpoint
  - IPC `Stop()` double-close panic (wrap in `closeOnce.Do`)
  - Redact env variable values in API responses
  - Auth middleware returns JSON errors (not plain text)
  - Tighten file permissions 0644 → 0600 across config, state, approval, crash logs
  - Shared panic recovery helper (`internal/panicutil`) added to all unprotected goroutines
