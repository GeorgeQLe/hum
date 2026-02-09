# devctl

A terminal UI for managing multiple local dev servers from a single pane.

![Node.js](https://img.shields.io/badge/node-%3E%3D18-brightgreen)

## Overview

devctl reads app definitions from `apps.json` at the project root and presents a split-screen TUI: a sidebar listing all registered apps with live status indicators, and a scrollable log pane showing output from the selected process. A command line at the bottom accepts commands to control apps.

## Installation

### Run directly (no install)

```sh
./devctl.mjs
```

### Install globally from local directory

```sh
npm install -g .
devctl  # now available anywhere
```

### Install globally from npm

```sh
npm install -g devctl
devctl
```

### Publish to npm (maintainers)

```sh
npm login
npm publish
```

## Usage

```sh
devctl              # launch the TUI
devctl --start-all  # launch and start all apps immediately
```

Requires a TTY terminal with at least 40 columns and 12 rows.

## Status Indicators

The sidebar displays a status dot for each app:

| Indicator | Color  | Meaning |
| --------- | ------ | ------- |
| ● | Green  | Running |
| ● | Red    | Crashed |
| ● | Yellow | Stopping |
| ○ | Dim    | Stopped |

## Commands

| Command              | Description                              |
| -------------------- | ---------------------------------------- |
| `start <name\|all>`  | Start an app or all apps                 |
| `stop <name\|all>`   | Stop an app or all apps                  |
| `restart <name\|all>`| Restart an app or all apps               |
| `status [name]`      | Show app status table                    |
| `ports`              | Check port availability                  |
| `scan`               | Auto-detect apps in project tree         |
| `add`                | Add a new app interactively              |
| `remove <name>`      | Remove an app from config                |
| `reload`             | Reload config from apps.json             |
| `autorestart [name]` | View/toggle auto-restart status          |
| `list`               | List configured apps with details        |
| `help`               | Show available commands                  |
| `quit`               | Stop all apps and exit                   |

## Keyboard Shortcuts

### Global

| Key         | Action                     |
| ----------- | -------------------------- |
| `Ctrl+C`    | Quit devctl                |
| `PgUp/PgDn` | Scroll log output          |

### Sidebar (when focused)

| Key              | Action                                    |
| ---------------- | ----------------------------------------- |
| `Tab`            | Switch focus to command line              |
| `Up/Down`, `j/k` | Navigate apps                             |
| `s`              | Start selected app                        |
| `S`              | Stop selected app                         |
| `r`              | Restart selected app                      |
| `Shift+R`        | Restart all running apps                  |
| `Enter`          | Switch to command line                    |

### Command Line (when focused)

| Key           | Action                         |
| ------------- | ------------------------------ |
| `Tab`         | Autocomplete / toggle sidebar  |
| `/`           | Enter log search mode          |
| `Ctrl+F`      | Enter log search mode          |
| `Up/Down`     | Navigate command history       |
| `Ctrl+U`      | Clear entire line              |
| `Ctrl+W`      | Delete word before cursor      |
| `Ctrl+A/Home` | Jump to beginning of line      |
| `Ctrl+E/End`  | Jump to end of line            |

### Log Search Mode

| Key       | Action                              |
| --------- | ----------------------------------- |
| `n`       | Jump to next match                  |
| `N`       | Jump to previous match              |
| `Enter`   | Exit search (keep highlights)       |
| `Esc`     | Exit search                         |
| `Ctrl+U`  | Clear search pattern                |

### Scan Mode

| Key       | Action                              |
| --------- | ----------------------------------- |
| `Tab`     | Toggle focus: candidate list / README |
| `Space`   | Toggle selection of current app     |
| `a`       | Select all / deselect all           |
| `j/k`     | Navigate candidates                 |
| `Enter`   | Confirm and add selected apps       |
| `Esc`     | Cancel scan                         |
| `PgUp/Dn` | Scroll README pane                  |

## App Configuration

Apps are stored in `apps.json` at the project root. Each entry has:

```json
{
  "name": "my-app",
  "dir": "apps/my-app",
  "command": "pnpm dev",
  "ports": [3000]
}
```

- **name** - Display name (unique)
- **dir** - Directory relative to project root
- **command** - Shell command to run
- **ports** - Array of ports the app listens on (used for conflict detection)

## Scanning for Apps

The `scan` command walks the project tree looking for `package.json` files with a `scripts.dev` entry. It then:

1. Identifies server frameworks (Next.js, Vite, Wrangler, Expo, nodemon, tsx watch)
2. Filters out library watchers (`tsc --watch`, `tsup --watch`)
3. Detects the package manager (pnpm, yarn, npm) from lockfiles
4. Detects ports from script args, config files, or framework defaults
5. Excludes monorepo roots when sub-apps are present
6. Skips already-registered apps

Each candidate is presented interactively for confirmation before being added.

## Process Management

- Processes run in detached process groups
- Stop sends SIGTERM with a 5-second timeout, then SIGKILL
- Stdout and stderr are captured and displayed in the log pane
- Crashed processes are indicated with a red status dot

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
  "name": "my-app",
  "dir": "apps/my-app",
  "command": "pnpm dev",
  "ports": [3000],
  "autoRestart": true,
  "restartDelay": 3000,
  "maxRestarts": 5
}
```

- **autoRestart** - Enable automatic restart on crash (default: false)
- **restartDelay** - Milliseconds to wait before restarting (default: 3000)
- **maxRestarts** - Maximum restart attempts before giving up (default: 5)

Use `autorestart` command to view status or toggle at runtime:
- `autorestart` - Show status for all apps
- `autorestart <name>` - Toggle auto-restart for an app
- `autorestart <name> on|off` - Enable/disable explicitly

## Config Reload

The `reload` command re-reads `apps.json` without restarting devctl:

- **Added apps** appear in the sidebar immediately
- **Removed apps** prompt to stop if running, then are removed
- **Changed apps** prompt to restart with the new config

This allows you to edit the config file externally and apply changes without losing log history or restarting running apps.

## Log Search

Press `/` or `Ctrl+F` (when command line is empty) to search the current log buffer:

- Type your search pattern (supports regex)
- Matches are highlighted in yellow, current match in magenta
- Press `n` to jump to next match, `N` for previous
- Press `Enter` or `Esc` to exit search mode

The search is case-insensitive and updates as you type.
