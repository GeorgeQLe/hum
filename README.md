# devctl

A terminal UI for managing multiple local dev servers from a single pane.

![Go](https://img.shields.io/badge/go-%3E%3D1.24-00ADD8)

## Overview

devctl reads app definitions from `apps.json` at the project root and presents a split-screen TUI: a sidebar listing all registered apps with live status indicators, and a scrollable log pane showing output from the selected process. A command line at the bottom accepts commands to control apps.

## Installation

### Quick install

```sh
bash install.sh
```

This builds the binary with `go build` and installs it to `~/.local/bin/devctl`. Your shell PATH is updated automatically.

### Manual build

```sh
make build
cp devctl ~/.local/bin/   # or anywhere on your PATH
```

### From source

```sh
go build -o devctl .
./devctl
```

**Uninstall:** `rm ~/.local/bin/devctl`

## Claude Code Integration

To install devctl as a Claude Code skill:

```sh
bash install-skill.sh
```

This creates a skill at `~/.claude/skills/devctl/SKILL.md`. Use `--agents-md` to also generate an `AGENTS.md` file in the current directory.

## Usage

```sh
devctl              # launch the TUI
devctl --start-all  # launch and start all apps immediately
devctl --restore    # restore previous session (restart apps that were running when you last quit)
```

Recommended: a TTY terminal with at least 40 columns and 12 rows for best display.

## CLI Subcommands

These commands communicate with a running TUI instance via IPC (Unix socket), so you can control devctl from another terminal:

```sh
devctl ping                          # check if a TUI instance is running
devctl status                        # show app names, statuses, PIDs, and ports
devctl start <name|all>              # start an app (auto-resolves port conflicts)
devctl stop <name|all>               # stop an app
devctl restart <name|all>            # restart an app
devctl add <dir>                     # add an app from a directory
devctl add <dir> --name my-app       # override detected name
devctl add <dir> --command "npm dev" # override detected command
devctl add <dir> --ports 3000,3001   # specify ports
devctl add <dir> --start             # start the app immediately after adding
devctl scan                          # auto-detect apps in project tree
devctl scan --write                  # add detected apps to apps.json
devctl scan --json                   # output detected apps as JSON
devctl stats                         # show CPU, memory, peak, and uptime for all apps
devctl stats --watch                 # live refresh every 2 seconds
devctl stats --json                  # JSON output for scripting
devctl dev                           # development mode with auto-rebuild on source changes
```

## TUI Commands

| Command                | Description                            |
| ---------------------- | -------------------------------------- |
| `start <name\|all>`    | Start an app or all apps               |
| `stop <name\|all>`     | Stop an app or all apps                |
| `restart <name\|all>`  | Restart an app or all apps             |
| `status [name]`        | Show app status table                  |
| `ports`                | Check port availability                |
| `scan`                 | Auto-detect apps in project tree       |
| `add`                  | Add a new app interactively            |
| `remove <name>`        | Remove an app from config              |
| `reload`               | Reload config from apps.json           |
| `autorestart [name]`   | View/toggle auto-restart status        |
| `run <name> <type>`    | Run a custom command variant           |
| `top`                  | Live resource dashboard                |
| `pin <name>`           | Pin an app to the top of the sidebar   |
| `unpin <name>`         | Unpin an app                           |
| `export <name> [file]` | Export app logs to file                |
| `clear-errors [name\|all]` | Clear detected errors              |
| `watch [name] [on\|off]` | View/toggle file watching            |
| `list`                 | List configured apps with details      |
| `help`                 | Show available commands                |
| `quit`                 | Stop all apps and exit                 |

Commands accept `@group` targets (e.g., `start @frontend`, `stop @backend`).

## Keyboard Shortcuts

### Global

| Key              | Action                        |
| ---------------- | ----------------------------- |
| `Ctrl+C`         | Quit devctl                   |
| `Ctrl+B`         | Toggle sidebar visibility     |
| `PgUp/PgDn`      | Scroll log output (page)     |
| `Ctrl+J/K`       | Scroll log output (line)     |

### Sidebar (when focused)

| Key              | Action                        |
| ---------------- | ----------------------------- |
| `Tab`            | Switch focus to command line   |
| `Up/Down`, `j/k` | Navigate apps                |
| `s`              | Start selected app            |
| `S`              | Stop selected app             |
| `r`              | Restart selected app          |
| `R`              | Restart all running apps      |
| `p`              | Toggle pin on selected app    |
| `x`              | Toggle errors-only log view   |
| `e`              | Copy last error to clipboard  |
| `E`              | Copy all errors to clipboard  |
| `Enter`          | Switch to command line        |

### Command Line (when focused)

| Key           | Action                         |
| ------------- | ------------------------------ |
| `Tab`         | Autocomplete / toggle sidebar  |
| `/`           | Enter log search mode          |
| `Ctrl+F`      | Enter log search mode          |
| `f`           | Toggle filter mode             |
| `t`           | Toggle timestamps on log lines |
| `x`           | Toggle errors-only log view    |
| `e`           | Copy last error to clipboard   |
| `E`           | Copy all errors to clipboard   |
| `Up/Down`     | Navigate command history       |
| `Ctrl+U`      | Clear entire line              |
| `Ctrl+W`      | Delete word before cursor      |
| `Ctrl+A/Home` | Jump to beginning of line      |
| `Ctrl+E/End`  | Jump to end of line            |

### Log Search Mode

| Key       | Action                        |
| --------- | ----------------------------- |
| `n`       | Jump to next match            |
| `N`       | Jump to previous match        |
| `Enter`   | Exit search (keep highlights) |
| `Esc`     | Exit search                   |
| `Ctrl+U`  | Clear search pattern          |

### Filter Mode

Press `f` from the command line to enter filter mode. Type a regex pattern to filter log lines in real time. The active filter is shown as `[filter: <pattern>]` in the command line. Press `f` again or `Esc` to clear.

### Top Mode (live resource dashboard)

| Key       | Action             |
| --------- | ------------------ |
| `c`       | Sort by CPU        |
| `m`       | Sort by memory     |
| `n`       | Sort by name       |
| `u`       | Sort by uptime     |
| `r`       | Reverse sort order |
| `j/k`     | Navigate apps      |
| `Esc`/`q` | Exit top mode      |

### Error Stream Mode

Press `x` to enter error stream mode, which shows only detected errors for the selected app.

| Key       | Action                            |
| --------- | --------------------------------- |
| `j/k`     | Navigate errors                   |
| `Enter`   | Toggle expand/collapse error      |
| `e`       | Copy full error block to clipboard |
| `m`       | Copy error message to clipboard   |
| `l`       | Copy source location to clipboard |
| `c`       | Clear all errors for current app  |
| `PgUp/Dn` | Scroll error list                |
| `x`/`Esc` | Exit error stream mode           |

### Scan Mode

| Key        | Action                               |
| ---------- | ------------------------------------ |
| `Tab`      | Toggle focus: candidate list / README |
| `Space`    | Toggle selection of current app      |
| `a`        | Select all / deselect all            |
| `j/k`      | Navigate candidates                  |
| `Enter`    | Confirm and add selected apps        |
| `Esc`      | Cancel scan                          |
| `PgUp/Dn`  | Scroll README pane                   |

## App Configuration

Apps are stored in `apps.json` at the project root. Each entry has:

```json
{
  "name": "my-app",
  "dir": "apps/my-app",
  "command": "pnpm dev",
  "ports": [3000],
  "project": "monorepo",
  "autoStart": true,
  "autoRestart": true,
  "restartDelay": 3000,
  "maxRestarts": 5,
  "env": {
    "NODE_ENV": "development"
  },
  "dependsOn": ["api-server"],
  "group": "frontend",
  "healthCheck": {
    "url": "http://localhost:3000/health",
    "interval": 5000
  },
  "resourceLimits": {
    "maxCpu": 80.0,
    "maxMemoryMB": 512
  },
  "notifications": true,
  "pinned": true,
  "commands": {
    "dev": "npm run dev",
    "build": "npm run build",
    "test": "npm test"
  },
  "watch": {
    "paths": ["src"],
    "extensions": [".ts", ".go"],
    "ignore": ["node_modules", "dist"]
  }
}
```

| Field | Description |
| ----- | ----------- |
| **name** | Display name (unique, required) |
| **dir** | Directory relative to project root (required) |
| **command** | Shell command to run (required) |
| **ports** | Array of ports the app listens on (required) |
| **project** | Optional project grouping label |
| **autoStart** | Auto-start the app when devctl launches (default: false) |
| **autoRestart** | Restart on crash (default: false) |
| **restartDelay** | Milliseconds before restart (default: 3000) |
| **maxRestarts** | Max restart attempts before giving up (default: 5) |
| **env** | Extra environment variables passed to the process |
| **dependsOn** | App names that must start before this one |
| **group** | Group name for sidebar grouping and `@group` commands |
| **healthCheck** | HTTP health check config (`url` and `interval` in ms) |
| **resourceLimits** | CPU/memory thresholds for alerts (`maxCpu` %, `maxMemoryMB`) |
| **notifications** | Enable desktop notifications for crashes and resource alerts |
| **pinned** | Pin app to the top of the sidebar |
| **commands** | Named command variants, runnable via `run <name> <type>` |
| **watch** | File watching config for auto-restart on source changes (`paths`, `extensions`, `ignore`) |

## Groups & Dependencies

### Groups

Assign a `"group"` to apps in `apps.json` to organize the sidebar. Groups appear as section headers, and you can target an entire group with commands:

```sh
start @frontend    # start all apps in the "frontend" group
stop @backend      # stop all apps in the "backend" group
restart @workers
```

### Dependencies

Declare `"dependsOn"` to control startup order. Dependencies are resolved via topological sort — devctl starts dependencies first and detects cycles:

```json
{
  "name": "web",
  "dependsOn": ["api", "db"],
  ...
}
```

Starting `web` will automatically start `api` and `db` first. `start all` and `--restore` both respect dependency order.

## Health Checks

Configure an HTTP health check to monitor whether an app is actually serving:

```json
{
  "healthCheck": {
    "url": "http://localhost:3000/health",
    "interval": 5000
  }
}
```

- Polls the URL at the given interval (minimum 1 second, default 5 seconds)
- 2xx/3xx responses = healthy (green heart in sidebar)
- 4xx/5xx or connection error = unhealthy (red heart in sidebar)
- Starts polling when the app starts, stops when it stops

## Resource Monitoring

devctl tracks CPU and memory usage for all running processes (polled every 2 seconds via `ps`).

- **`top` command** — opens a live dashboard with sortable columns (CPU, memory, name, uptime). Sort with `c`/`m`/`n`/`u`, reverse with `r`, exit with `q`.
- **`devctl stats`** — view resource statistics from another terminal. Use `--watch` for live updates or `--json` for scripting.
- **Threshold alerts** — configure `resourceLimits` per app to get notified when CPU or memory exceeds a threshold. A red triangle appears in the sidebar, and desktop notifications are sent if `notifications` is enabled.

```json
{
  "resourceLimits": {
    "maxCpu": 80.0,
    "maxMemoryMB": 512
  },
  "notifications": true
}
```

## Filtering

Press `f` from the command line to enter filter mode. Type a regex pattern to show only matching log lines. The filter applies in real time and is shown in the command bar. Press `f` again or `Esc` to clear the filter.

## Error Detection

devctl scans process output for common error patterns (`ERROR`, `Failed`, `Exception`, `TypeError`, `FATAL`, stack traces, etc.) and:

- Shows an error count with a red `!` indicator in the sidebar
- Displays a notification banner when errors are detected
- Press `e` to copy the last error to your clipboard, `E` to copy all errors
- Use `clear-errors [name|all]` to reset error counts

## Session Restore

When devctl quits, it saves the list of running apps to `.devctl-state.json`. On next launch with `--restore`, it restarts those apps in dependency order:

```sh
devctl --restore
```

Apps that are no longer in `apps.json` or have port conflicts are skipped with a warning.

## IPC

devctl exposes a Unix socket so external tools and terminals can interact with a running instance:

```sh
devctl ping       # health check — is devctl running?
devctl status     # list all apps with status, PID, ports
devctl add ./app  # add an app to the running instance
devctl stats      # resource usage snapshot
```

The socket is created at `$TMPDIR/devctl-sockets/devctl-<hash>.sock` (scoped to the project root). Stale sockets are cleaned up automatically.

## Status Indicators

The sidebar displays a status dot for each app:

| Indicator | Color  | Meaning  |
| --------- | ------ | -------- |
| ●         | Green  | Running  |
| ●         | Red    | Crashed  |
| ●         | Yellow | Stopping |
| ○         | Dim    | Stopped  |

Additional indicators: `!` (errors detected), ♥ (health check status), ▲ (resource threshold exceeded).

## Scanning for Apps

The `scan` command walks the project tree looking for `package.json` files with a `scripts.dev` entry. It then:

1. Identifies server frameworks (Next.js, Vite, Wrangler, Expo, nodemon, tsx watch)
2. Filters out library watchers (`tsc --watch`, `tsup --watch`)
3. Detects the package manager (pnpm, yarn, npm) from lockfiles
4. Detects ports from script args, config files, or framework defaults
5. Excludes monorepo roots when sub-apps are present
6. Skips already-registered apps

Each candidate is presented interactively for confirmation before being added.

## Port Conflict Handling

When starting an app, devctl checks if the required ports are available. If a port is in use, you get interactive resolution options:

**If the port is used by another devctl app:**
- `[r]` Restart the blocking app, then start this one
- `[a]` Use an alternative free port (if available)
- `[s]` Start anyway (may fail)
- `[c]` Cancel

**If the port is used by an external process:**
- `[k]` Kill the external process and start
- `[a]` Use an alternative free port (if available)
- `[s]` Start anyway (may fail)
- `[c]` Cancel

The `ports` command shows current port status and identifies which process owns each port.

## System Logs

The first entry in the sidebar (labeled "devctl") shows the system log. This captures:

- App start/stop events
- Status summaries from `start all`
- Command output and errors
- Scan results

Select it to view devctl's internal activity separate from app logs.

## Auto-Restart

Apps can be configured to automatically restart when they crash:

```json
{
  "autoRestart": true,
  "restartDelay": 3000,
  "maxRestarts": 5
}
```

- **autoRestart** — Enable automatic restart on crash (default: false)
- **restartDelay** — Milliseconds to wait before restarting (default: 3000)
- **maxRestarts** — Maximum restart attempts before giving up (default: 5)

Use `autorestart` command to view status or toggle at runtime:
- `autorestart` — Show status for all apps
- `autorestart <name>` — Toggle auto-restart for an app
- `autorestart <name> on|off` — Enable/disable explicitly

## File Watching

Configure file watching to auto-restart an app when source files change:

```json
{
  "watch": {
    "paths": ["src", "lib"],
    "extensions": [".ts", ".go"],
    "ignore": ["node_modules", "dist"]
  }
}
```

- **paths** — Directories to watch, relative to the app's `dir`
- **extensions** — File extensions to trigger on (e.g., `[".ts", ".go"]`)
- **ignore** — Glob patterns or directory names to exclude

Use `watch` in the TUI to view status or toggle at runtime:
- `watch` — Show watch status for all apps
- `watch <name>` — Toggle watching for an app
- `watch <name> on|off` — Enable/disable explicitly

## Config Reload

The `reload` command re-reads `apps.json` without restarting devctl:

- **Added apps** appear in the sidebar immediately
- **Removed apps** prompt to stop if running, then are removed
- **Changed apps** prompt to restart with the new config

devctl also watches `apps.json` for external changes and reloads automatically.

## Log Search

Press `/` or `Ctrl+F` (when command line is empty) to search the current log buffer:

- Type your search pattern (supports regex)
- Matches are highlighted in yellow, current match in magenta
- Press `n` to jump to next match, `N` for previous
- Press `Enter` or `Esc` to exit search mode

The search is case-insensitive and updates as you type.

## Testing

```sh
make test        # unit tests (fast, no process spawning)
make test-e2e    # end-to-end tests (spawns real processes, ~40s)
```

The e2e suite lives in `internal/e2e/` behind a `//go:build e2e` tag so it never runs during `go test ./...`. It exercises the full lifecycle through the Manager, HTTP API, IPC, and Health Checker using real shell commands as mock applications.

## Process Management

- Processes run via `sh -c` in detached process groups
- Stop sends SIGTERM to the process group with a 5-second timeout, then SIGKILL
- Stdout and stderr are captured and displayed in the log pane
- Crashed processes are indicated with a red status dot
- Environment: inherits parent env plus per-app `env` vars

## envsafe — Encrypted Secrets Manager

envsafe is a local-first encrypted environment variable manager that integrates with devctl. Secrets are encrypted with AES-256-GCM using an Argon2id-derived key and stored in a `.envsafe/` directory.

### Quick Start

```sh
# Initialize a vault in your project
envsafe init

# Store a secret
envsafe set API_KEY sk-secret-value

# Retrieve it
envsafe get API_KEY

# List all keys (values hidden)
envsafe list

# Export as KEY=VALUE pairs
envsafe env
```

### devctl Integration

Add `vault_env` to your app config in `apps.json` to auto-inject secrets at startup:

```json
{
  "name": "api-server",
  "dir": "apps/api",
  "command": "npm run dev",
  "ports": [3000],
  "vault_env": "development"
}
```

devctl will unlock the vault (using the OS keychain or `ENVSAFE_PASSWORD` env var) and merge secrets into the process environment. Plain-text `env` values take precedence over vault values.

### Multi-Environment Support

Secrets are organized by environment:

```sh
envsafe set -e production DATABASE_URL "postgres://..."
envsafe set -e staging DATABASE_URL "postgres://staging..."
envsafe list -e production
```

### Secret Rotation

Rotate a secret while preserving its history:

```sh
envsafe rotate API_KEY sk-new-value
```

Previous values are stored in an audit trail within the vault.

### Team Sharing

Share secrets with teammates using X25519 envelope encryption:

```sh
envsafe user add alice@example.com    # generates a key pair
envsafe user list                     # show team members
envsafe user role alice@example.com admin
```

### Password Management

Vault passwords must be at least 8 characters.

```sh
envsafe passwd       # change the vault master password
envsafe unlock       # unlock and cache password in OS keychain
envsafe lock         # lock the vault and clear cached password
```

### Backup & Restore

```sh
envsafe backup                          # creates envsafe-backup-<timestamp>.tar.gz
envsafe backup -o my-backup.tar.gz      # custom output path
envsafe restore my-backup.tar.gz        # restore from archive
```

### Audit Log

envsafe maintains an append-only audit log of all vault operations:

```sh
envsafe audit                           # view all audit entries
envsafe audit --action set              # filter by action (set, get, delete, rotate)
envsafe audit --user alice@example.com  # filter by user
envsafe audit --env production          # filter by environment
envsafe audit --since 2024-01-01        # filter entries after date (YYYY-MM-DD)
envsafe audit --format json             # JSON output
envsafe audit --format csv              # CSV output
```

### Interactive Browser

```sh
envsafe browse       # TUI vault explorer with 4-view navigation
```

### All Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize a new encrypted vault |
| `set` | Store a secret |
| `get` | Retrieve a secret |
| `list` | List secret keys (values hidden) |
| `rm` | Remove a secret |
| `env` | Export all secrets as KEY=VALUE |
| `rotate` | Rotate a secret (preserves history) |
| `passwd` | Change the vault master password |
| `unlock` | Unlock vault and cache password in keychain |
| `lock` | Lock vault and clear cached password |
| `backup` | Back up vault to compressed archive |
| `restore` | Restore vault from backup archive |
| `browse` | Interactive TUI vault explorer |
| `user` | Manage team members (add/list/remove/role) |
| `audit` | View audit log |
| `share` | Create a one-time share link (requires server) |
| `login` | Authenticate with server (**experimental** — not yet functional) |
| `serve` | Start the server (**experimental** — not yet functional) |

### Security

- **Encryption:** AES-256-GCM with Argon2id key derivation. Vault files are safe to commit to git.
- **Backup permissions:** Backup archives are created with `0600` permissions (owner-only read/write). Restore limits extracted file sizes to 100 MB each to prevent decompression bombs.
- **Password requirements:** Minimum 8 characters enforced during vault init and password change.
- **Share safety:** The `share` command blocks non-localhost HTTP servers by default. Use `--insecure` to override. Share expiry is capped at 7 days.
- **Server JWT:** The `serve` command generates a random JWT secret if none is provided via `--jwt-secret`. Tokens will not survive server restarts unless a stable secret is configured.
- **Memory hygiene:** Password byte slices are zeroed after conversion to string to minimize exposure window.
