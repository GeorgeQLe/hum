# devctl - TODO

## New Features

### High Priority

- [ ] **Config File Watcher** - Auto-reload `apps.json` when it changes on disk using `fs.watch()`, eliminating need for manual `reload` command

- [ ] **Environment Variables** - Support per-app environment variables in config:
  ```json
  { "name": "api", "env": { "NODE_ENV": "development", "DEBUG": "true" } }
  ```

- [ ] **App Dependencies** - Add `dependsOn: ["app-name"]` to config so apps start in correct order (e.g., database before API server). Topological sort for start order.

- [ ] **Log Timestamps** - Optional toggle (`t` key or config) to prefix log lines with timestamps for debugging timing issues

- [ ] **Session Persistence** - Save which apps were running on exit to `.devctl-state.json`; add `--restore` flag to restore previous session

### Medium Priority

- [ ] **Health Check Support** - Add optional config field:
  ```json
  { "healthCheck": { "url": "http://localhost:3000/health", "interval": 5000 } }
  ```
  Show health status indicator alongside running status

- [ ] **Log Export** - Add `export <app> [file]` command to save logs to disk (default: `<app>-<timestamp>.log`)

- [ ] **App Groups** - Support grouping apps:
  ```json
  { "name": "api", "group": "backend" }
  ```
  Then use `start @backend` to start all apps in a group

- [ ] **Log Filtering** - Add filter mode (separate from search) that hides non-matching lines. Toggle with `f` key.

### Low Priority

- [ ] **Resource Monitoring** - Show CPU/memory usage in `status` command output (via `/proc/<pid>/stat` on Linux, `ps` on macOS)

- [ ] **Desktop Notifications** - Notify on crash via `notify-send` (Linux), `osascript` (macOS), or `powershell` (Windows)

- [ ] **Favorite/Pin Apps** - Pin frequently used apps to top of sidebar

- [ ] **Custom Commands** - Support custom scripts beyond just `dev`:
  ```json
  { "commands": { "dev": "pnpm dev", "build": "pnpm build" } }
  ```

---

## Refactoring

### Code Organization

- [ ] **Split into modules** - Break up the monolithic file:
  ```
  devctl.mjs           # Entry point, CLI args
  lib/terminal.mjs     # ANSI codes, cursor control, box drawing
  lib/render.mjs       # UI rendering (sidebar, log pane, command line)
  lib/process.mjs      # Process spawn, stop, restart, auto-restart
  lib/config.mjs       # Load/save/validate apps.json
  lib/state.mjs        # Centralized state management
  lib/commands.mjs     # Command handlers
  lib/input.mjs        # Keyboard input handling
  ```

- [ ] **Consolidate state** - Replace scattered global variables with single state object:
  ```javascript
  const state = {
    apps: [],
    procs: new Map(),
    ui: { selectedIdx: 0, focusArea: 'command', cmdInput: '', ... },
    search: null,
    scan: null,
    config: { logMaxLines: 5000, ... }
  };
  ```

### DRY Improvements

- [ ] **Port conflict handling** - Extract common `promptPortConflict(options)` function from duplicated code at lines 1509-1616 (devctl-managed vs external process handling)

- [ ] **Render row abstraction** - Create shared base for `renderSidebarRow`, `renderLogRow`, `renderScanCandidateRow`, `renderScanReadmeRow`

- [ ] **Question prompts** - Create reusable `promptChoice({ message, choices })` helper for interactive prompts

### Configuration

- [ ] **Externalize magic numbers** - Make constants configurable via `apps.json` or `devctl.config.json`:
  ```javascript
  const DEFAULTS = {
    LOG_MAX_LINES: 5000,
    MAX_ERRORS_PER_APP: 50,
    RENDER_THROTTLE_MS: 16,
    SHUTDOWN_TIMEOUT_MS: 10000,
    KILL_TIMEOUT_MS: 5000,
    AUTO_RESTART_DELAY: 3000,
    MAX_RESTARTS: 5
  };
  ```

### Performance & Quality

- [ ] **Parallel restart** - Update `cmdRestart('all')` at line 2816 to run restarts in parallel (like `cmdStart` does)

- [ ] **JSDoc types** - Add type annotations for better editor support:
  ```javascript
  /** @typedef {{ name: string, dir: string, command: string, ports: number[] }} AppConfig */
  ```

- [ ] **Error boundaries** - Improve error handling in functions that silently catch (e.g., `loadConfig`, `walkForPackageJsons`)

- [ ] **Unit tests** - Add test coverage for:
  - Config validation
  - Port detection
  - ANSI stripping/wrapping
  - Command parsing

---

## Bugs / Tech Debt

- [ ] **lsof dependency** - `getPortOwnerInfo()` uses `lsof` which may not be available on all systems. Add fallback or graceful degradation.

- [ ] **PROJECT_ROOT assumption** - `PROJECT_ROOT` is set to parent of script location. May not work for all install scenarios.

- [ ] **Scroll position on app switch** - Scroll position persists per-buffer but `follow` mode can be confusing when switching between apps

---

## Completed

- [x] **Quick Keyboard Shortcuts** - Added `s`/`S`/`r` shortcuts in sidebar mode for start/stop/restart selected app
