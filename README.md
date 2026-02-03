# devctl

A terminal UI for managing multiple local dev servers from a single pane.

![Node.js](https://img.shields.io/badge/node-%3E%3D18-brightgreen)

## Overview

devctl reads app definitions from `apps.json` at the project root and presents a split-screen TUI: a sidebar listing all registered apps with live status indicators, and a scrollable log pane showing output from the selected process. A command line at the bottom accepts commands to control apps.

## Usage

```sh
./scripts/devctl.mjs              # launch the TUI
./scripts/devctl.mjs --start-all  # launch and start all apps immediately
```

Requires a TTY terminal with at least 40 columns and 12 rows.

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
| `list`               | List configured apps with details        |
| `help`               | Show available commands                  |
| `quit`               | Stop all apps and exit                   |

## Keyboard Shortcuts

| Key              | Action                                 |
| ---------------- | -------------------------------------- |
| `Tab`            | Toggle between sidebar and command line|
| `Up/Down`, `j/k` | Navigate apps in sidebar              |
| `PgUp/PgDn`     | Scroll log output                      |
| `Ctrl+C`         | Quit                                  |
| `Ctrl+U`         | Clear command line                     |
| `Ctrl+W`         | Delete word                            |

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
- Port availability is checked before starting (advisory, not blocking)
- Stdout and stderr are captured and displayed in the log pane
- Crashed processes are indicated with a red status dot
