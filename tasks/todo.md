# devctl - Audit PR List

50 PRs across 10 tiers. Highest-leverage changes are PRs #1-3 (structural), #5-10 (reliability), and #11-17 (DX).

---

## Tier 1 -- Structural / Showstopper Fixes

- [ ] **PR #1: Split `main.go` (21K lines) into proper Cobra command files** - Extract each subcommand (`ping`, `status`, `add`, `stats`, `scan`, `dev`, `start`, `stop`, `restart`) into `cmd/devctl/` files following standard Cobra patterns. Single biggest maintainability blocker.

- [ ] **PR #2: Split `tui/model.go` (3,444 lines) into composable sub-models** - Refactor into sub-models with their own `Update()` / `View()` methods. Reduces cognitive load, enables independent testing, prevents merge conflicts.

- [ ] **PR #3: Add CI pipeline (GitHub Actions)** - `go vet`, `staticcheck`, `go test -race ./...`, `golangci-lint`, build matrix (linux/darwin/windows), release automation via GoReleaser.

- [ ] **PR #4: Return 501 Not Implemented for stub endpoints** - `internal/server/handlers.go` has ~8 handler stubs (register, login, TOTP, secrets CRUD) returning 200 OK with placeholder bodies. Clients think operations succeeded when nothing happened.

---

## Tier 2 -- Reliability & Correctness

- [ ] **PR #5: Fix event channel overflow dropping critical state changes** - `process/manager.go:627-644` has a 1024-event buffer. When full, critical events (crash, restart) are silently dropped. Add priority queue, backpressure, and surface dropped-event count in TUI.

- [ ] **PR #6: Persist logs to disk with rotation** - Logs are memory-only with a hard 5000-line cap (`logbuffer.go:10`). TUI crash = total log loss. Add per-app log files under `.devctl/logs/`, configurable retention, and `devctl logs --tail`.

- [ ] **PR #7: Fix graceful shutdown ordering** - Session state saved *before* processes stopped (`model.go:2078`). Fix: stop processes -> confirm shutdown -> save final state. Also increase 10s timeout for large app sets.

- [ ] **PR #8: Fix panic recovery to restore terminal state** - `main.go:194-200` catches panics but `p.Kill()` doesn't guarantee terminal restoration. Add `tea.ExecProcess` cleanup, stop managed processes, remove IPC socket, print stack trace to file.

- [ ] **PR #9: Integrate health checks with auto-restart logic** - `health/checker.go` polls HTTP endpoints but results never feed into restart decisions. Wire health status into supervisor: unhealthy for N checks -> restart.

- [ ] **PR #10: Fix IPC socket security -- use XDG_RUNTIME_DIR** - IPC socket in `/tmp/devctl-sockets/` readable by any local user. Move to `$XDG_RUNTIME_DIR/devctl/` (Linux) or `~/Library/Caches/devctl/` (macOS) with 0700 permissions. Add client auth via nonce.

---

## Tier 3 -- Developer Experience

- [ ] **PR #11: Add `devctl init` interactive scaffolding** - Detect package managers, ask for app names/ports/commands, generate `apps.json`, optionally run `devctl scan` to auto-detect services.

- [ ] **PR #12: Add `devctl validate` for config linting** - Validate JSON syntax, check fields against schema, detect dependency cycles, validate port ranges, warn about missing health checks.

- [ ] **PR #13: Add real-time log streaming to HTTP API** - `api/handlers.go:164` has a stub comment. Implement proper SSE push from log buffer so external tools can tail logs.

- [ ] **PR #14: Add `devctl logs` CLI subcommand** - `devctl logs <app> [--follow] [--lines N] [--since 5m]` connecting via IPC, streaming logs to stdout. Essential for piping into grep/jq workflows.

- [ ] **PR #15: Support JSONC or YAML for apps.json** - JSON has no comments. Support JSONC or YAML as alternative formats with auto-detection.

- [ ] **PR #16: Add `devctl doctor` diagnostic command** - Verify `lsof` availability, check port conflicts, validate config, test IPC socket, verify dependencies, check disk space. Output pass/fail checklist.

- [ ] **PR #17: Add shell completions (bash/zsh/fish)** - `devctl completion bash|zsh|fish` using Cobra's built-in generator. Include completions for app names in `start/stop/restart`.

---

## Tier 4 -- Performance & Scalability

- [ ] **PR #18: Cache TUI layout computation** - `model.go` calls `recalcLayout()` on every `View()`. Cache on `WindowSizeMsg` only. Memoize rendered sidebar/log content between frames.

- [ ] **PR #19: Increase event buffer and add backpressure** - Event channel is 1024 entries. Increase to 8192, add ring-buffer semantics, implement backpressure via `select` with `default` to log drops.

- [ ] **PR #20: Lazy-load process logs in TUI** - Only render selected app's log pane. For sidebar apps, show last-line preview but skip full ANSI rendering until focused.

- [ ] **PR #21: Fix port scan range (hardcoded 9999 ceiling)** - `process/port.go:83` scans `basePort..9999`. Should scan to 65535 or configurable upper bound.

---

## Tier 5 -- Security Hardening

- [ ] **PR #22: Validate and sandbox shell commands from apps.json** - `process/manager.go:239` passes `command` to `sh -c` verbatim. Add allowlist validation, `--dry-run` preview, and warnings for shell metacharacters.

- [ ] **PR #23: Rotate API bearer tokens** - `api/discovery.go` writes a static token that never rotates. Add token expiry (24h), auto-rotation on restart, `devctl token rotate`, and token scoping.

- [ ] **PR #24: Add rate limiting to IPC and HTTP API** - No rate limiting exists. Add per-client limits (100 req/s IPC, 60 req/min HTTP mutations) with 429 responses.

- [ ] **PR #25: Audit logging for all secret operations** - `vault/vault.go` has no audit trail. Add structured audit entries for access, modify, rotate, share, delete. Feed into `vault/audit/audit.go`.

---

## Tier 6 -- Observability & Debugging

- [ ] **PR #26: Add structured logging with slog** - Replace mixed `log.Printf` / `fmt.Fprintf(os.Stderr)` / silent swallows with Go 1.21+ `log/slog`. Add levels, JSON output mode, `--verbose` / `--debug` flags.

- [ ] **PR #27: Add `--debug` flag with verbose output** - Enable debug-level logging, print IPC messages, show config load steps, report event channel utilization, dump process manager state.

- [ ] **PR #28: Surface dropped events and error counts in TUI** - Dropped events print to stderr (invisible in TUI). Add status bar indicator for dropped events, error buffer size, memory usage, IPC queue depth.

- [ ] **PR #29: Add `devctl debug dump` for bug reports** - Dump Go version, OS, terminal size, running apps, port allocations, IPC status, event queue depth, memory usage, last 100 log lines per app. Anonymize secrets.

---

## Tier 7 -- Testing & Quality

- [ ] **PR #30: Add integration test suite for TUI** - Use `teatest` for mode transitions, keyboard handling, layout at various terminal sizes, and approval flow.

- [ ] **PR #31: Add end-to-end test with real processes** - Create temp `apps.json` with `echo` processes, start devctl, verify apps start, send IPC commands, verify responses, check graceful shutdown. Run in CI.

- [ ] **PR #32: Add fuzz testing for config parser** - `go test -fuzz` for malformed JSON, extreme values, unicode app names, empty arrays, circular dependencies.

- [ ] **PR #33: Add benchmark tests for hot paths** - `Benchmark*` tests for log buffer append/trim, TUI view rendering, event channel throughput, ANSI sanitization.

---

## Tier 8 -- Ecosystem & Distribution

- [ ] **PR #34: Add Dockerfile and docker-compose example** - Multi-stage Dockerfile + `docker-compose.yml` showing devctl managing services in a container.

- [ ] **PR #35: Add Homebrew formula** - Homebrew tap with formula for `brew install devctl`. Pre-built bottles for common platforms.

- [ ] **PR #36: Add GoReleaser config for cross-platform releases** - `.goreleaser.yml` for linux/darwin/windows (amd64/arm64), GitHub releases with checksums, changelog, Homebrew formula publishing.

- [ ] **PR #37: Add man pages and `--help` improvements** - Detailed `--help` with examples, `devctl help <topic>` for concepts, generate man pages from Cobra.

---

## Tier 9 -- Feature Gaps

- [ ] **PR #38: Add `.env` file support per app** - Per-app `.env` paths in config, auto-reload on change, variable expansion, integration with envsafe vault injection.

- [ ] **PR #39: Add pre/post lifecycle hooks** - `preStart`, `postStart`, `preStop`, `postStop` hooks running shell commands. Use case: migrations before server start, cache warm after start.

- [ ] **PR #40: Add webhook/notification integration** - Slack webhook on crash, generic webhook POST on state change, `notify-send` on Linux, configurable notification rules per app.

- [ ] **PR #41: Add `devctl exec <app> <command>`** - Run one-off command with app's env/cwd/port config. Essential for migrations, seeds, REPL.

- [ ] **PR #42: Add dependency health gating** - Wait for dependency health check to pass before starting dependent app. Configurable timeout and failure behavior.

- [ ] **PR #43: Add process resource limits** - Optional `resources: { maxMemoryMB: 512, cpuLimit: 50 }` in apps.json. Use `ulimit` (macOS) or cgroups (Linux).

- [ ] **PR #44: Add log export (JSON, file, S3)** - `devctl logs export --format json|text --output ./logs/` for batch export, `--stream-to` for continuous export.

- [ ] **PR #45: Add HTTP API approval endpoint** - `POST /api/approvals/{id}/approve` and `POST /api/approvals/{id}/deny` for CI/CD pipelines and external tools.

---

## Tier 10 -- Code Cleanup / Tech Debt

- [ ] **PR #46: Fix all silent error swallows** - 8+ locations where errors assigned to `_` in critical paths: `json.Marshal` in IPC (`ipc/server.go:150,162`), `os.WriteFile` for config backup (`config/config.go:232`), `syscall.Kill` (`manager.go:392,402`).

- [ ] **PR #47: Deduplicate supervisor event loops** - `dev/supervisor.go` has two nearly identical event loops (lines 96-164 and 322-380). Extract into shared `runEventLoop()`.

- [ ] **PR #48: Add golangci-lint config and fix all warnings** - Add `.golangci.yml` with reasonable defaults, fix existing warnings, add to CI.

- [ ] **PR #49: Add Go doc comments to all exported symbols** - Add `godoc`-compatible comments to all public APIs across all packages.

- [ ] **PR #50: Remove dead code and TODO stubs** - Multiple TODO comments and placeholder directories (`server/handlers/`, `envsafe-tui/views/`, incomplete DB queries). Implement or remove.

---

## Previously Completed

- [x] **Config File Watcher** - Auto-reload `apps.json` when it changes on disk
- [x] **Log Timestamps** - Toggle (`t` key) to prefix log lines with timestamps
- [x] **Session Persistence** - Save running apps on exit to `.devctl-state.json`; `--restore` flag
- [x] **Quick Keyboard Shortcuts** - `s`/`S`/`r` shortcuts in sidebar for start/stop/restart
