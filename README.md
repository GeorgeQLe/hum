# devctl

A terminal UI for managing multiple local dev servers from a single pane.

![Go](https://img.shields.io/badge/go-%3E%3D1.22-00ADD8)

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

## Usage

```sh
devctl              # launch the TUI
devctl --start-all  # launch and start all apps immediately
devctl --restore    # restore previous session (restart apps that were running when you last quit)
```

Requires a TTY terminal with at least 40 columns and 12 rows.

## CLI Subcommands

These commands communicate with a running TUI instance via IPC (Unix socket), so you can control devctl from another terminal:

```sh
devctl ping                          # check if a TUI instance is running
devctl status                        # show app names, statuses, PIDs, and ports
devctl add <dir>                     # add an app from a directory
devctl add <dir> --name my-app       # override detected name
devctl add <dir> --command "npm dev" # override detected command
devctl add <dir> --ports 3000,3001   # specify ports
devctl add <dir> --start             # start the app immediately after adding
devctl stats                         # show CPU, memory, peak, and uptime for all apps
devctl stats --watch                 # live refresh every 2 seconds
devctl stats --json                  # JSON output for scripting
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
| `list`                 | List configured apps with details      |
| `help`                 | Show available commands                |
| `quit`                 | Stop all apps and exit                 |

Commands accept `@group` targets (e.g., `start @frontend`, `stop @backend`).

## Keyboard Shortcuts

### Global

| Key          | Action              |
| ------------ | ------------------- |
| `Ctrl+C`     | Quit devctl         |
| `PgUp/PgDn`  | Scroll log output   |

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
  }
}
```

| Field | Description |
| ----- | ----------- |
| **name** | Display name (unique, required) |
| **dir** | Directory relative to project root (required) |
| **command** | Shell command to run (required) |
| **ports** | Array of ports the app listens on (required) |
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

devctl scans process output for common error patterns (`ERROR`, `Exception`, `TypeError`, `FATAL`, stack traces, etc.) and:

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

## Process Management

- Processes run via `sh -c` in detached process groups
- Stop sends SIGTERM to the process group with a 5-second timeout, then SIGKILL
- Stdout and stderr are captured and displayed in the log pane
- Crashed processes are indicated with a red status dot
- Environment: inherits parent env plus per-app `env` vars
