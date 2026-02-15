#!/usr/bin/env bash
set -euo pipefail

SKILL_DIR="$HOME/.claude/skills/devctl"
SKILL_FILE="$SKILL_DIR/SKILL.md"
AGENTS_MD=false

for arg in "$@"; do
    case "$arg" in
        --agents-md) AGENTS_MD=true ;;
        *)
            echo "Usage: bash install-skill.sh [--agents-md]" >&2
            echo "  --agents-md  Also write AGENTS.md to the current directory" >&2
            exit 1
            ;;
    esac
done

# --- Skill body (agent-agnostic) ---
read -r -d '' SKILL_BODY << 'BODY' || true
# devctl — Local Dev Environment Manager

devctl is a TUI process orchestrator for running multiple dev servers from a single terminal. It reads `apps.json` to know which services to run.

## Checking if devctl is available

```bash
# Is the binary installed?
command -v devctl

# Is the TUI already running for this project?
devctl ping
```

## Starting devctl

```bash
devctl                # launch TUI (interactive)
devctl --start-all    # launch TUI and start all apps (respects dependency order)
devctl --restore      # launch TUI and restore previous session state
```

## Remote commands (run from another terminal while TUI is running)

```bash
devctl ping                          # check if TUI is running
devctl status                        # list apps with status, PID, ports
devctl add <dir> [--name n] [--command c] [--ports p1,p2] [--start]
devctl start <name|all>              # start an app (auto-resolves port conflicts)
devctl stop <name|all>               # stop an app
devctl restart <name|all>            # restart an app
devctl stats [--watch] [--json]      # resource usage (CPU, memory, uptime)
devctl scan [--json] [--write]       # auto-detect apps in project tree
```

## TUI commands (type `:` in the TUI to open the command line)

| Command | Description |
|---|---|
| `start <name\|all\|@group>` | Start an app, all apps, or a group |
| `stop <name\|all\|@group>` | Stop an app, all apps, or a group |
| `restart <name\|all\|@group>` | Restart an app, all apps, or a group |
| `status [name]` | Show status, PID, uptime, CPU/MEM, ports |
| `list` | List all configured apps with details |
| `ports` | Check port availability and ownership |
| `scan` | Auto-detect apps in project tree |
| `add` | Interactive wizard to add a new app |
| `remove <name>` | Remove an app from config |
| `reload` | Reload apps.json (detects changes) |
| `run <name> [command-key]` | Run a custom command from the app's `commands` map |
| `export <name> [file]` | Export app logs to file |
| `top` | Live resource dashboard |
| `autorestart [name] [on\|off]` | View or toggle auto-restart |
| `clear-errors [name\|all]` | Reset error counters |
| `pin <name>` / `unpin <name>` | Pin/unpin app to top of sidebar |
| `help` | Show all commands and keyboard shortcuts |

## Group targeting

Apps can have a `group` field. Target all apps in a group with `@group`:

```
start @frontend
stop @backend
restart @workers
```

## Dependencies

The `dependsOn` field lists apps that must be running first. When you start an app, devctl auto-starts its dependencies in topological order.

```json
{ "name": "web", "dependsOn": ["api", "db"] }
```

Starting `web` will start `db` and `api` first if they aren't already running.

## apps.json format

`apps.json` is an array of app objects at the project root:

```json
[
  {
    "name": "my-app",
    "dir": "apps/my-app",
    "command": "pnpm dev",
    "ports": [3000],
    "group": "frontend",
    "dependsOn": ["api"],
    "autoRestart": true,
    "restartDelay": 3000,
    "maxRestarts": 5,
    "env": { "NODE_ENV": "development" },
    "healthCheck": { "url": "http://localhost:3000/health", "interval": 5000 },
    "resourceLimits": { "maxCpu": 80.0, "maxMemoryMB": 512 },
    "notifications": true,
    "pinned": true,
    "commands": { "build": "npm run build", "test": "npm test" }
  }
]
```

### Required fields

| Field | Type | Description |
|---|---|---|
| `name` | string | Unique app identifier |
| `dir` | string | Working directory (relative to project root) |
| `command` | string | Shell command to run (e.g. `pnpm dev`) |
| `ports` | int[] | Ports the app listens on |

### Optional fields

| Field | Type | Description |
|---|---|---|
| `group` | string | Group name for `@group` targeting |
| `dependsOn` | string[] | Apps that must start first |
| `autoRestart` | bool | Auto-restart on crash |
| `restartDelay` | int | Delay before restart (ms) |
| `maxRestarts` | int | Max consecutive restarts |
| `env` | object | Environment variables |
| `healthCheck` | object | `{ "url": "...", "interval": ms }` |
| `resourceLimits` | object | `{ "maxCpu": %, "maxMemoryMB": MB }` |
| `notifications` | bool | Desktop notifications on crash/restart |
| `pinned` | bool | Pin to top of sidebar |
| `commands` | object | Named commands (used with `run`) |

## Common workflows

### Setting up a new project (no apps.json yet)

**Always prefer auto-detection over hand-writing apps.json.**

1. Run `devctl scan` to preview what the scanner finds
2. Run `devctl scan --write` to auto-detect apps and write them to `apps.json`
3. Review the generated `apps.json` — add anything the scanner missed via `devctl add <dir>` or by editing `apps.json` directly
4. Start everything: `devctl --start-all`

**Never hand-write a large apps.json from scratch.** The scanner detects package managers, dev scripts, and ports automatically. Manual entries are error-prone and hard to validate.

### Add a new service to the project

Preferred one-liner: `devctl add <dir> --start` — auto-detects name, command, and ports, then starts the app.

If auto-detection doesn't cover your case:
1. Check what the scanner finds: `devctl scan`
2. Add manually with overrides: `devctl add <dir> --name myapp --command "node server.js" --ports 3000`
3. Or edit `apps.json` directly then `:reload` in the TUI
4. Start it: `devctl start <name>`

### Debug a crashing service

1. Check status: `status <name>` — look at exit code and restart count
2. Read the logs in the TUI (select the app in the sidebar)
3. Check port conflicts: `ports`
4. Check resource usage: `top` or `devctl stats`
5. Clear error state after fixing: `clear-errors <name>`

### Check resource usage

```bash
devctl stats              # one-shot from another terminal
devctl stats --watch      # live updating
devctl stats --json       # machine-readable
```

Or in the TUI: `top`

### Export logs for sharing

```
export <name>             # writes <name>-<timestamp>.log
export <name> output.log  # writes to specific file
```

## Agent workflow

When using devctl from an AI agent (e.g. Claude Code), follow this recommended flow:

1. **Register and start in one step**: `devctl add <dir> --start`
   - Auto-detects app name, command, and ports from `package.json`
   - When ports can't be detected, devctl assigns a free port and injects `PORT=<port>` env var
   - If the TUI is not running, `add` writes to `apps.json` directly (but can't `--start`)

2. **Control apps after registration**:
   - `devctl start <name|all>` — starts an app (auto-resolves port conflicts)
   - `devctl stop <name|all>` — stops an app
   - `devctl restart <name|all>` — restarts an app

3. **Manual overrides** when detection is wrong:
   - `--name <name>` — override detected app name
   - `--command <cmd>` — override detected command (e.g. `"node server.js"`)
   - `--ports <p1,p2>` — override detected ports

4. **Bulk setup**: `devctl scan --write` detects all apps in the project tree and writes them to `apps.json` with auto-assigned ports where needed.

5. **Check status**: `devctl status` shows all apps with their state, PID, and ports.
BODY

# --- Write SKILL.md ---
echo "Installing devctl skill for Claude Code..."
mkdir -p "$SKILL_DIR"

cat > "$SKILL_FILE" << SKILL
---
name: devctl
description: >-
  Manage local dev environments with devctl — start/stop/restart services,
  check status, manage ports, and monitor resources. Use when working on
  projects with apps.json or devctl configuration.
user-invocable: true
allowed-tools: Bash, Read, Grep, Glob
---
${SKILL_BODY}
SKILL

echo "Installed skill to $SKILL_FILE"

# --- Optionally write AGENTS.md ---
if [ "$AGENTS_MD" = true ]; then
    echo "$SKILL_BODY" > AGENTS.md
    echo "Wrote AGENTS.md to $(pwd)/AGENTS.md"
fi

echo ""
echo "Done! In Claude Code, type /devctl to invoke the skill."
