# devctl - TODO

## New Features

### High Priority

- [x] **Config File Watcher** - Auto-reload `apps.json` when it changes on disk, eliminating need for manual `reload` command

- [ ] **Environment Variables** - Support per-app environment variables in config:
  ```json
  { "name": "api", "env": { "NODE_ENV": "development", "DEBUG": "true" } }
  ```

- [ ] **App Dependencies** - Add `dependsOn: ["app-name"]` to config so apps start in correct order (e.g., database before API server). Topological sort for start order.

- [x] **Log Timestamps** - Optional toggle (`t` key or config) to prefix log lines with timestamps for debugging timing issues

- [x] **Session Persistence** - Save which apps were running on exit to `.devctl-state.json`; add `--restore` flag to restore previous session

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

- [ ] **Resource Monitoring** - Show CPU/memory usage in `status` command output (via `ps` on macOS/Linux)

- [ ] **Desktop Notifications** - Notify on crash via `osascript` (macOS) or `notify-send` (Linux)

- [ ] **Favorite/Pin Apps** - Pin frequently used apps to top of sidebar

- [ ] **Custom Commands** - Support custom scripts beyond just `dev`:
  ```json
  { "commands": { "dev": "pnpm dev", "build": "pnpm build" } }
  ```

- [x] **Quick Keyboard Shortcuts** - Added `s`/`S`/`r` shortcuts in sidebar mode for start/stop/restart selected app

---

## Bugs / Tech Debt

- [ ] **lsof dependency** - `getPortOwnerInfo()` uses `lsof` which may not be available on all systems. Add fallback or graceful degradation.

---

## Performance & Quality

- [ ] **Unit tests** - Expand test coverage for Go packages (`internal/config`, `internal/process`, `internal/tui`)
