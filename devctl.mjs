#!/usr/bin/env node

import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import readline from 'readline';
import net from 'net';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const PROJECT_ROOT = path.resolve(__dirname, '..');
const CONFIG_PATH = path.join(PROJECT_ROOT, 'apps.json');

// ── ANSI & Terminal Control ─────────────────────────────

const COLORS = [
  '\x1b[36m', '\x1b[33m', '\x1b[35m', '\x1b[32m',
  '\x1b[34m', '\x1b[91m', '\x1b[96m', '\x1b[93m',
];
const RESET   = '\x1b[0m';
const BOLD    = '\x1b[1m';
const DIM     = '\x1b[2m';
const INVERSE = '\x1b[7m';
const RED     = '\x1b[31m';
const GREEN   = '\x1b[32m';
const YELLOW  = '\x1b[33m';

const SYSTEM_NAME = 'devctl';

const ALT_SCREEN_ON  = '\x1b[?1049h';
const ALT_SCREEN_OFF = '\x1b[?1049l';
const CURSOR_HIDE    = '\x1b[?25l';
const CURSOR_SHOW    = '\x1b[?25h';
const CLEAR_SCREEN   = '\x1b[2J';

function moveTo(row, col) {
  return `\x1b[${row + 1};${col + 1}H`;
}

const BOX = {
  TL: '\u250c', TR: '\u2510', BL: '\u2514', BR: '\u2518',
  H: '\u2500', V: '\u2502',
  ML: '\u251c', MR: '\u2524', MB: '\u2534',
};

// ── State ───────────────────────────────────────────────

let apps = [];
const procs = new Map();
let shuttingDown = false;

// TUI state
let selectedIdx = 0;
let focusArea = 'command'; // 'sidebar' | 'command'
const logBuffers = new Map();
let cmdInput = '';
let cmdCursor = 0;
let cmdHistory = [];
let historyIdx = -1;
let historyTemp = '';
let questionMode = null;
let processing = false;
let tuiReady = false;
let terminalCleaned = false;

// Layout cache
let layout = null;

// Render throttling
let renderTimer = null;
let fullRenderNeeded = false;

// Tab completion state
let tabState = null;

// Scan mode state
let scanMode = null;
// When active: { candidates, cursorIdx, selected: Set, readmeCache: Map, readmeScrollPos, candidateScroll }

// Search mode state
let searchMode = null;
// When active: { pattern, matches: [{lineIdx, start, end}], matchIdx, regex }

// Error detection state
let errorBuffers = new Map(); // Map<appName, { errors: [], lastNotified }>
let errorNotification = null; // { message, fadeTimer }

// Config file watcher state
let configWatcher = null;
let ignoreNextConfigChange = false;

// Scan skip directories
const SCAN_SKIP_DIRS = new Set([
  'node_modules', '.git', '.next', 'dist', 'build', '.turbo',
  '_archive', 'clones', 'starters', 'archive',
]);

// Error patterns to detect
const ERROR_PATTERNS = [
  /\bERROR\b/i,
  /\bError:/,
  /\bException\b/,
  /\bFailed\b/i,
  /\bFATAL\b/i,
  /\bTypeError\b|\bReferenceError\b|\bSyntaxError\b/,
  /at\s+\S+\s+\([^)]+:\d+:\d+\)/,  // Stack trace lines
  /^\s+at\s+/,                      // Indented stack traces
];

// ── Config Manager ──────────────────────────────────────

function loadConfig() {
  if (!fs.existsSync(CONFIG_PATH)) {
    saveConfig([]);
    return [];
  }
  try {
    const raw = fs.readFileSync(CONFIG_PATH, 'utf-8');
    const data = JSON.parse(raw);
    if (!Array.isArray(data)) {
      return [];
    }
    const valid = [];
    for (const entry of data) {
      const err = validateAppEntry(entry);
      if (!err) valid.push(entry);
    }
    return valid;
  } catch {
    return [];
  }
}

function saveConfig(data) {
  const clean = data.map(a => {
    const entry = {
      name: a.name,
      dir: a.dir,
      command: a.command,
      ports: a.ports,
    };
    // Preserve auto-restart settings if present
    if (a.autoRestart !== undefined) entry.autoRestart = a.autoRestart;
    if (a.restartDelay !== undefined) entry.restartDelay = a.restartDelay;
    if (a.maxRestarts !== undefined) entry.maxRestarts = a.maxRestarts;
    return entry;
  });
  ignoreNextConfigChange = true;
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(clean, null, 2) + '\n');
}

function setupConfigWatcher() {
  if (configWatcher) return;

  let debounceTimer = null;

  configWatcher = fs.watch(CONFIG_PATH, (eventType) => {
    if (eventType !== 'change') return;
    if (ignoreNextConfigChange) {
      ignoreNextConfigChange = false;
      return;
    }

    // Debounce: file saves often trigger multiple events
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      debounceTimer = null;
      log(`${DIM}Config file changed, reloading...${RESET}`);
      cmdReload();
    }, 100);
  });

  configWatcher.on('error', () => {
    // Silently ignore watch errors
  });
}

function closeConfigWatcher() {
  if (configWatcher) {
    configWatcher.close();
    configWatcher = null;
  }
}

function validateAppEntry(entry) {
  if (!entry || typeof entry !== 'object') return 'not an object';
  if (!entry.name || typeof entry.name !== 'string') return 'missing or invalid "name"';
  if (!entry.dir || typeof entry.dir !== 'string') return 'missing or invalid "dir"';
  if (!entry.command || typeof entry.command !== 'string') return 'missing or invalid "command"';
  if (
    !Array.isArray(entry.ports) ||
    entry.ports.length === 0 ||
    !entry.ports.every(p => typeof p === 'number' && Number.isInteger(p) && p > 0 && p < 65536)
  ) {
    return '"ports" must be a non-empty array of integers 1\u201365535';
  }
  // Validate optional auto-restart fields
  if (entry.autoRestart !== undefined && typeof entry.autoRestart !== 'boolean') {
    return '"autoRestart" must be a boolean';
  }
  if (entry.restartDelay !== undefined) {
    if (typeof entry.restartDelay !== 'number' || entry.restartDelay < 0) {
      return '"restartDelay" must be a non-negative number';
    }
  }
  if (entry.maxRestarts !== undefined) {
    if (typeof entry.maxRestarts !== 'number' || entry.maxRestarts < 0 || !Number.isInteger(entry.maxRestarts)) {
      return '"maxRestarts" must be a non-negative integer';
    }
  }
  return null;
}

// ── Port Checker ────────────────────────────────────────

function isPortFree(port) {
  return new Promise(resolve => {
    const srv = net.createServer();
    srv.once('error', () => resolve(false));
    srv.once('listening', () => srv.close(() => resolve(true)));
    srv.listen(port, '127.0.0.1');
  });
}

async function getPortOwnerInfo(port) {
  return new Promise(resolve => {
    const proc = spawn('lsof', ['-i', `:${port}`, '-P', '-n', '-sTCP:LISTEN'], {
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    let stdout = '';
    proc.stdout.on('data', data => { stdout += data.toString(); });

    proc.on('error', () => resolve(null));
    proc.on('close', code => {
      if (code !== 0 || !stdout.trim()) {
        resolve(null);
        return;
      }

      // Parse lsof output (skip header line)
      const lines = stdout.trim().split('\n');
      if (lines.length < 2) {
        resolve(null);
        return;
      }

      // Format: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME
      const parts = lines[1].split(/\s+/);
      if (parts.length < 3) {
        resolve(null);
        return;
      }

      resolve({
        command: parts[0],
        pid: parseInt(parts[1], 10),
        user: parts[2],
      });
    });
  });
}

function findDevctlOwner(pid) {
  for (const [name, entry] of procs.entries()) {
    if (entry.status === 'running' && entry.proc && entry.proc.pid === pid) {
      return name;
    }
  }
  return null;
}

async function suggestAlternativePorts(basePorts) {
  const suggestions = [];
  const offsets = [1, 10, 100];

  for (const port of basePorts) {
    let suggested = null;
    for (const offset of offsets) {
      const candidate = port + offset;
      if (candidate < 65536 && await isPortFree(candidate)) {
        suggested = candidate;
        break;
      }
    }
    suggestions.push({ original: port, suggested });
  }

  return suggestions;
}

async function checkPortConflicts(appList) {
  // Check all ports for all apps in parallel
  const allPorts = new Set();
  for (const app of appList) {
    for (const port of app.ports) {
      allPorts.add(port);
    }
  }

  const portStatus = new Map();
  await Promise.all(
    [...allPorts].map(async port => {
      const free = await isPortFree(port);
      portStatus.set(port, free);
    })
  );

  // Categorize apps into conflict-free and conflicting
  const conflictFree = [];
  const conflicting = [];

  for (const app of appList) {
    const hasConflict = app.ports.some(p => !portStatus.get(p));
    if (hasConflict) {
      conflicting.push(app);
    } else {
      conflictFree.push(app);
    }
  }

  return { conflictFree, conflicting };
}

async function killExternalProcess(pid) {
  return new Promise(resolve => {
    try {
      process.kill(pid, 'SIGTERM');
    } catch (e) {
      if (e.code === 'EPERM') {
        resolve({ success: false, reason: 'permission' });
        return;
      }
      resolve({ success: false, reason: 'error' });
      return;
    }

    // Wait for process to exit, then SIGKILL if needed
    let attempts = 0;
    const checkInterval = setInterval(() => {
      attempts++;
      try {
        // Check if process still exists (signal 0 doesn't kill, just checks)
        process.kill(pid, 0);
        if (attempts >= 10) {
          // Still alive after 2.5s, send SIGKILL
          clearInterval(checkInterval);
          try {
            process.kill(pid, 'SIGKILL');
            setTimeout(() => resolve({ success: true }), 500);
          } catch {
            resolve({ success: true }); // Already gone
          }
        }
      } catch {
        // Process no longer exists
        clearInterval(checkInterval);
        resolve({ success: true });
      }
    }, 250);
  });
}

// ── ANSI Stripping ──────────────────────────────────────

function stripAnsi(str) {
  return str.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '');
}

function sanitizeLine(str) {
  return str
    // Strip cursor-positioning / screen-control CSI sequences:
    //   \x1b[...H (cursor move), \x1b[...J (erase display),
    //   \x1b[...K (erase line), \x1b[...A/B/C/D/E/F/G (cursor motion),
    //   \x1b[...S/T (scroll), \x1b[...?...h/l (private modes like alt screen)
    .replace(/\x1b\[\??[0-9;]*[HABCDEFGJKSTfhlr]/g, '')
    // Strip OSC sequences (title setting, hyperlinks): \x1b]...BEL/ST
    .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '')
    // Strip bare carriage returns (progress-line overwrites)
    .replace(/\r/g, '')
    // Strip other non-CSI escapes: \x1b(B, \x1b>, etc.
    .replace(/\x1b[^[]\S?/g, '');
}

function wrapLine(str, width) {
  if (width <= 0) return [str];
  const stripped = stripAnsi(str);
  if (stripped.length <= width) return [str];

  const segments = [];
  let vis = 0;
  let segStart = 0;
  let i = 0;
  let lastSpace = -1;      // index in str of last space
  let lastSpaceVis = -1;   // visual position of last space

  while (i < str.length) {
    if (str[i] === '\x1b') {
      const m = str.slice(i).match(/^\x1b\[[0-9;]*[a-zA-Z]/);
      if (m) { i += m[0].length; continue; }
    }
    if (str[i] === ' ') {
      lastSpace = i;
      lastSpaceVis = vis;
    }
    vis++;
    i++;
    if (vis >= width && i < str.length) {
      // Break at last space if available, otherwise break at current position
      if (lastSpace > segStart) {
        segments.push(str.slice(segStart, lastSpace));
        segStart = lastSpace + 1; // skip the space
        vis = vis - lastSpaceVis - 1;
      } else {
        segments.push(str.slice(segStart, i));
        segStart = i;
        vis = 0;
      }
      lastSpace = -1;
      lastSpaceVis = -1;
    }
  }

  if (segStart < str.length) {
    segments.push(str.slice(segStart));
  }

  return segments.length > 0 ? segments : [str];
}

function getLogTextWidth() {
  return layout ? layout.logInner - 1 : 79;
}

function getDisplayLines(buf, textWidth) {
  const result = [];
  for (const line of buf.lines) {
    const wrapped = wrapLine(line, textWidth);
    for (const seg of wrapped) result.push(seg);
  }
  return result;
}

function getDisplayLineCount(buf, textWidth) {
  if (textWidth <= 0) return buf.lines.length;
  let count = 0;
  for (const line of buf.lines) {
    const visLen = stripAnsi(line).length;
    count += Math.max(1, Math.ceil(visLen / textWidth));
  }
  return count;
}

function fitToWidth(str, width) {
  if (width <= 0) return '';
  const stripped = stripAnsi(str);
  if (stripped.length <= width) {
    return str + ' '.repeat(width - stripped.length);
  }
  // Truncate to visible width
  let vis = 0;
  let i = 0;
  while (i < str.length && vis < width) {
    if (str[i] === '\x1b') {
      const m = str.slice(i).match(/^\x1b\[[0-9;]*[a-zA-Z]/);
      if (m) { i += m[0].length; continue; }
    }
    vis++;
    i++;
  }
  return str.slice(0, i) + RESET;
}

// ── Layout Calculator ───────────────────────────────────
//
// Row 0:          top border
// Rows 1..H-4:   main content (sidebar | log pane)
// Row H-3:       divider
// Row H-2:       command line
// Row H-1:       bottom border

function calcLayout() {
  const rows = process.stdout.rows || 24;
  const cols = process.stdout.columns || 80;

  if (rows < 12 || cols < 40) return null;

  const nameSource = scanMode ? scanMode.candidates : apps;
  const maxName = nameSource.length > 0
    ? Math.max(...nameSource.map(a => a.name.length))
    : 4;
  const extraWidth = scanMode ? 4 : 0;
  const sidebarInner = Math.min(
    Math.max(16, maxName + 6 + extraWidth),
    Math.floor(cols * 0.35),
  );
  const logInner = cols - sidebarInner - 3;

  return {
    rows, cols,
    sidebarInner,
    logInner,
    mainTop: 1,
    mainBottom: rows - 4,
    mainHeight: rows - 4,
    dividerRow: rows - 3,
    cmdRow: rows - 2,
    bottomRow: rows - 1,
  };
}

// ── Clipboard Helper ────────────────────────────────────

function copyToClipboard(text) {
  return new Promise((resolve, reject) => {
    const platform = process.platform;
    let cmd, args;

    if (platform === 'darwin') {
      cmd = 'pbcopy';
      args = [];
    } else if (platform === 'win32') {
      cmd = 'clip';
      args = [];
    } else {
      // Linux - check if we're in WSL
      const isWSL = fs.existsSync('/proc/version') &&
        fs.readFileSync('/proc/version', 'utf-8').toLowerCase().includes('microsoft');
      if (isWSL) {
        cmd = 'clip.exe';
        args = [];
      } else {
        // Try xclip first, fall back to xsel
        cmd = 'xclip';
        args = ['-selection', 'clipboard'];
      }
    }

    const proc = spawn(cmd, args, { stdio: ['pipe', 'ignore', 'ignore'] });
    proc.on('error', (err) => {
      // Try xsel as fallback on Linux
      if (cmd === 'xclip') {
        const xsel = spawn('xsel', ['--clipboard', '--input'], { stdio: ['pipe', 'ignore', 'ignore'] });
        xsel.on('error', () => reject(new Error('No clipboard utility found')));
        xsel.on('close', code => code === 0 ? resolve() : reject(new Error('xsel failed')));
        xsel.stdin.write(text);
        xsel.stdin.end();
      } else {
        reject(err);
      }
    });
    proc.on('close', code => code === 0 ? resolve() : reject(new Error(`${cmd} failed`)));
    proc.stdin.write(text);
    proc.stdin.end();
  });
}

// ── Error Detection ─────────────────────────────────────

function getErrorBuffer(name) {
  if (!errorBuffers.has(name)) {
    errorBuffers.set(name, { errors: [], lastNotified: 0 });
  }
  return errorBuffers.get(name);
}

function matchesErrorPattern(line) {
  const stripped = stripAnsi(line);
  return ERROR_PATTERNS.some(pattern => pattern.test(stripped));
}

function detectAndCaptureError(name, lineIdx) {
  const buf = getLogBuffer(name);
  const errBuf = getErrorBuffer(name);

  // Capture from the error line until a blank line or end
  const errorLines = [];
  for (let i = lineIdx; i < buf.lines.length; i++) {
    const line = buf.lines[i];
    const stripped = stripAnsi(line).trim();
    if (stripped.length === 0) break;
    errorLines.push(line);
  }

  if (errorLines.length > 0) {
    errBuf.errors.push({
      timestamp: Date.now(),
      lines: errorLines,
    });

    // Show notification if viewing this app
    const selectedName = getSelectedBufName();
    if (selectedName === name) {
      showErrorNotification();
    }

    // Limit stored errors
    if (errBuf.errors.length > 50) {
      errBuf.errors.shift();
    }
  }
}

function showErrorNotification() {
  // Clear existing fade timer
  if (errorNotification?.fadeTimer) {
    clearTimeout(errorNotification.fadeTimer);
  }

  errorNotification = {
    message: 'Error detected! [e] copy',
    fadeTimer: setTimeout(() => {
      errorNotification = null;
      renderBottomBar();
    }, 5000),
  };

  renderBottomBar();
}

function getAppErrorCount(name) {
  const errBuf = errorBuffers.get(name);
  return errBuf ? errBuf.errors.length : 0;
}

function clearErrors(name) {
  if (name === 'all') {
    errorBuffers.clear();
  } else if (errorBuffers.has(name)) {
    errorBuffers.delete(name);
  }
}

function getLastErrorText(name) {
  const errBuf = errorBuffers.get(name);
  if (!errBuf || errBuf.errors.length === 0) return null;
  const lastError = errBuf.errors[errBuf.errors.length - 1];
  return lastError.lines.map(stripAnsi).join('\n');
}

function getAllErrorsText(name) {
  const errBuf = errorBuffers.get(name);
  if (!errBuf || errBuf.errors.length === 0) return null;
  return errBuf.errors
    .map((e, i) => `--- Error ${i + 1} ---\n` + e.lines.map(stripAnsi).join('\n'))
    .join('\n\n');
}

// ── Log Buffer Manager ─────────────────────────────────

const LOG_MAX_LINES = 5000;

function getLogBuffer(name) {
  if (!logBuffers.has(name)) {
    logBuffers.set(name, { lines: [], scrollPos: 0, follow: true });
  }
  return logBuffers.get(name);
}

function getLogViewHeight() {
  if (!layout) return 10;
  return layout.mainHeight - 1;
}

function appendLog(name, text, isStderr = false) {
  const buf = getLogBuffer(name);
  const rawLines = text.toString().split(/\r?\n|\r/);

  for (const line of rawLines) {
    if (line.length === 0) continue;
    const clean = sanitizeLine(line);
    if (clean.length === 0) continue;
    const lineIdx = buf.lines.length;
    buf.lines.push(isStderr ? `${DIM}${clean}${RESET}` : clean);

    // Check for error patterns
    if (matchesErrorPattern(clean)) {
      detectAndCaptureError(name, lineIdx);
    }
  }

  if (buf.lines.length > LOG_MAX_LINES) {
    const excess = buf.lines.length - LOG_MAX_LINES;
    buf.lines.splice(0, excess);
    buf.scrollPos = Math.max(0, buf.scrollPos - excess);
  }

  if (buf.follow) {
    const displayCount = getDisplayLineCount(buf, getLogTextWidth());
    buf.scrollPos = Math.max(0, displayCount - getLogViewHeight());
  }

  if (selectedIdx > 0 && apps[selectedIdx - 1]?.name === name) {
    scheduleLogRender();
  }
}

function log(msg) {
  const buf = getLogBuffer(SYSTEM_NAME);
  const lines = msg.split('\n');
  for (const line of lines) {
    buf.lines.push(line);
  }
  if (buf.lines.length > LOG_MAX_LINES) {
    const excess = buf.lines.length - LOG_MAX_LINES;
    buf.lines.splice(0, excess);
    buf.scrollPos = Math.max(0, buf.scrollPos - excess);
  }
  if (buf.follow) {
    const displayCount = getDisplayLineCount(buf, getLogTextWidth());
    buf.scrollPos = Math.max(0, displayCount - getLogViewHeight());
  }
  if (selectedIdx === 0) scheduleLogRender();
}

// ── Render Scheduling ───────────────────────────────────

function scheduleLogRender() {
  if (!tuiReady) return;
  if (renderTimer) return;
  renderTimer = setTimeout(() => {
    renderTimer = null;
    if (fullRenderNeeded) {
      fullRenderNeeded = false;
      renderFull();
    } else {
      renderLogPane();
      renderCommandLine();
    }
  }, 16);
}

function scheduleFullRender() {
  if (!tuiReady) return;
  fullRenderNeeded = true;
  if (renderTimer) clearTimeout(renderTimer);
  renderTimer = setTimeout(() => {
    renderTimer = null;
    fullRenderNeeded = false;
    renderFull();
  }, 0);
}

// ── Render Engine ───────────────────────────────────────

function renderFull() {
  if (!layout) {
    process.stdout.write(
      CLEAR_SCREEN + moveTo(0, 0) +
      'Terminal too small. Please resize to at least 40\u00d712.',
    );
    return;
  }

  const { rows, cols, sidebarInner, logInner, mainTop, mainBottom,
          dividerRow, cmdRow, bottomRow } = layout;
  let buf = CURSOR_HIDE;

  // Top border
  buf += moveTo(0, 0);
  const titleText = ` ${BOLD}devctl${RESET} `;
  const titleVisLen = 8; // " devctl "
  const topFill = cols - 2 - 1 - titleVisLen;
  buf += BOX.TL + BOX.H + titleText;
  buf += BOX.H.repeat(Math.max(0, topFill)) + BOX.TR;

  // Pre-compute display lines for log pane
  const logBuf = getLogBuffer(getSelectedBufName());
  const displayLines = getDisplayLines(logBuf, logInner - 1);

  // Main content rows
  for (let r = mainTop; r <= mainBottom; r++) {
    const rowIdx = r - mainTop;
    buf += moveTo(r, 0);
    buf += BOX.V;
    buf += renderSidebarRow(rowIdx, sidebarInner);
    buf += RESET + BOX.V;
    buf += renderLogRow(rowIdx, logInner, displayLines, logBuf.scrollPos);
    buf += RESET + BOX.V;
  }

  // Divider row
  buf += moveTo(dividerRow, 0);
  buf += BOX.ML + BOX.H.repeat(sidebarInner) + BOX.MB;
  buf += BOX.H.repeat(logInner) + BOX.MR;

  // Command line row
  buf += moveTo(cmdRow, 0);
  buf += BOX.V + renderCmdContent(cols - 2) + BOX.V;

  // Bottom border with hints
  buf += moveTo(bottomRow, 0);
  const hints = getHints();
  const hintsVis = stripAnsi(hints);
  const hintFill = cols - 2 - 3 - hintsVis.length;
  if (hintFill >= 0) {
    buf += BOX.BL + BOX.H + ' ' + hints + ' ' + BOX.H.repeat(hintFill) + BOX.BR;
  } else {
    buf += BOX.BL + BOX.H.repeat(cols - 2) + BOX.BR;
  }

  buf += positionCmdCursor() + CURSOR_SHOW;
  process.stdout.write(buf);
}

function renderSidebarRow(rowIdx, width) {
  if (scanMode) return renderScanCandidateRow(rowIdx, width);

  if (rowIdx === 0) {
    const style = focusArea === 'sidebar' ? BOLD : DIM;
    return fitToWidth(` ${style}APPS${RESET}`, width);
  }

  // Row 1: devctl system entry
  if (rowIdx === 1) {
    const isSelected = selectedIdx === 0;
    const prefix = isSelected ? ' \u25b8 ' : '   ';
    const name = SYSTEM_NAME;
    const padLen = width - 3 - name.length;
    const padding = padLen > 0 ? ' '.repeat(padLen) : '';
    if (isSelected && focusArea === 'sidebar') {
      return `${INVERSE}${prefix}${name}${padding}${RESET}`;
    }
    if (isSelected) {
      return `${BOLD}${prefix}${name}${RESET}${padding}`;
    }
    return `${DIM}${prefix}${name}${padding}${RESET}`;
  }

  // Row 2+: apps
  const appIdx = rowIdx - 2;
  if (appIdx < 0 || appIdx >= apps.length) {
    return ' '.repeat(width);
  }

  const app = apps[appIdx];
  const isSelected = (rowIdx - 1) === selectedIdx;
  const entry = procs.get(app.name);
  const status = entry?.status || 'stopped';

  // Dot indicator
  const filled = status === 'running' || status === 'crashed' || status === 'stopping';
  const dotChar = filled ? '\u25cf' : '\u25cb';
  const dotColor = status === 'running' ? GREEN
    : status === 'crashed' ? RED
    : status === 'stopping' ? YELLOW
    : DIM;

  // Error indicator
  const errorCount = getAppErrorCount(app.name);
  const errorIndicator = errorCount > 0 ? `${RED}!${RESET}` : '';
  const errorVisLen = errorCount > 0 ? 1 : 0;

  // Truncate name if needed (width - 3 prefix - 2 dot+space - error indicator)
  const maxNameLen = width - 5 - errorVisLen;
  let name = app.name;
  if (name.length > maxNameLen) {
    name = name.slice(0, Math.max(1, maxNameLen - 1)) + '\u2026';
  }

  const prefix = isSelected ? ' \u25b8 ' : '   ';
  const padLen = width - 3 - name.length - 2 - errorVisLen;
  const padding = padLen > 0 ? ' '.repeat(padLen) : '';

  if (isSelected && focusArea === 'sidebar') {
    return `${INVERSE}${prefix}${name}${padding} ${RESET}${errorIndicator}${dotColor}${dotChar}${RESET}`;
  }
  if (isSelected) {
    return `${BOLD}${prefix}${name}${RESET}${padding} ${errorIndicator}${dotColor}${dotChar}${RESET}`;
  }
  return `${prefix}${name}${padding} ${errorIndicator}${dotColor}${dotChar}${RESET}`;
}

function renderLogRow(rowIdx, width, displayLines, scrollPos) {
  if (scanMode) return renderScanReadmeRow(rowIdx, width);

  const app = getSelectedApp();

  if (rowIdx === 0) {
    if (selectedIdx === 0) {
      return fitToWidth(` ${BOLD}devctl${RESET}  ${DIM}system log${RESET}`, width);
    }
    if (!app) {
      return fitToWidth(` ${DIM}No apps configured${RESET}`, width);
    }
    const entry = procs.get(app.name);
    const status = entry?.status || 'stopped';
    const statusColor = status === 'running' ? GREEN
      : status === 'crashed' ? RED
      : status === 'stopping' ? YELLOW
      : DIM;
    const dot = (status === 'running' || status === 'crashed' || status === 'stopping')
      ? '\u25cf' : '\u25cb';
    const errorCount = getAppErrorCount(app.name);
    const errorSuffix = errorCount > 0 ? `  ${RED}${errorCount} error${errorCount > 1 ? 's' : ''}${RESET}` : '';
    const header = ` ${BOLD}${app.name}${RESET}  ${statusColor}${dot} ${status}${RESET}${errorSuffix}`;
    return fitToWidth(header, width);
  }

  const lineIdx = scrollPos + (rowIdx - 1);

  if (displayLines && lineIdx >= 0 && lineIdx < displayLines.length) {
    let line = displayLines[lineIdx];
    // Apply search highlighting if in search mode
    if (searchMode && searchMode.pattern) {
      line = highlightSearchInLine(line, lineIdx);
    }
    return fitToWidth(' ' + line, width);
  }

  return ' '.repeat(width);
}

function renderScanCandidateRow(rowIdx, width) {
  if (rowIdx === 0) {
    return fitToWidth(` ${BOLD}SCAN RESULTS${RESET}`, width);
  }

  const { candidates, cursorIdx, selected, candidateScroll } = scanMode;
  const appIdx = rowIdx - 1 + candidateScroll;
  if (appIdx < 0 || appIdx >= candidates.length) {
    return ' '.repeat(width);
  }

  const c = candidates[appIdx];
  const isCursor = appIdx === cursorIdx;
  const isChecked = selected.has(appIdx);
  const check = isChecked ? '[x]' : '[ ]';
  const arrow = isCursor ? '\u25b8' : ' ';

  const maxNameLen = width - 6; // arrow + space + check + space + name
  let name = c.name;
  if (name.length > maxNameLen) {
    name = name.slice(0, Math.max(1, maxNameLen - 1)) + '\u2026';
  }

  const padLen = width - 2 - 4 - name.length;
  const padding = padLen > 0 ? ' '.repeat(padLen) : '';
  const text = `${arrow} ${check} ${name}${padding}`;

  if (isCursor) {
    return `${INVERSE} ${text}${RESET}`;
  }
  if (isChecked) {
    return `${GREEN} ${text}${RESET}`;
  }
  return fitToWidth(` ${text}`, width);
}

function renderScanReadmeRow(rowIdx, width) {
  const { candidates, cursorIdx, readmeScrollPos } = scanMode;
  const c = candidates[cursorIdx];

  if (rowIdx === 0) {
    const nameStyle = scanMode.scanFocus === 'readme' ? INVERSE : BOLD;
    const header = ` ${nameStyle}${c.name}${RESET}  ${DIM}${c.dir}${RESET}`;
    return fitToWidth(header, width);
  }
  if (rowIdx === 1) {
    return fitToWidth(` ${DIM}command:${RESET} ${c.command}`, width);
  }
  if (rowIdx === 2) {
    return fitToWidth(` ${DIM}ports:${RESET}   ${c.ports.join(', ')}`, width);
  }
  if (rowIdx === 3) {
    return fitToWidth(` ${DIM}dev:${RESET}     ${c.devScript}`, width);
  }
  if (rowIdx === 4) {
    return BOX.H.repeat(width);
  }

  const readmeLines = getScanReadmeLines(c);
  const lineIdx = readmeScrollPos + (rowIdx - 5);
  if (lineIdx >= 0 && lineIdx < readmeLines.length) {
    return fitToWidth(' ' + readmeLines[lineIdx], width);
  }
  return ' '.repeat(width);
}

function getScanReadmeLines(candidate) {
  if (scanMode.readmeCache.has(candidate.dir)) {
    return scanMode.readmeCache.get(candidate.dir);
  }

  const readmePath = path.join(PROJECT_ROOT, candidate.dir, 'README.md');
  let lines;
  try {
    const content = fs.readFileSync(readmePath, 'utf-8');
    const textWidth = layout ? layout.logInner - 2 : 78;
    const rawLines = content.split('\n');
    lines = [];
    for (const line of rawLines) {
      const wrapped = wrapLine(line, textWidth);
      for (const seg of wrapped) lines.push(seg);
    }
  } catch {
    lines = ['No README.md found'];
  }

  scanMode.readmeCache.set(candidate.dir, lines);
  return lines;
}

function scrollScanReadme(delta) {
  const c = scanMode.candidates[scanMode.cursorIdx];
  const readmeLines = getScanReadmeLines(c);
  const viewHeight = layout ? layout.mainHeight - 5 : 10;
  const maxScroll = Math.max(0, readmeLines.length - viewHeight);

  scanMode.readmeScrollPos = Math.max(0, Math.min(maxScroll, scanMode.readmeScrollPos + delta));

  renderLogPane();
  renderCommandLine();
}

function renderLogPane() {
  if (!layout) return;
  const { logInner, mainTop, mainBottom, sidebarInner } = layout;

  // Pre-compute display lines for log pane
  const logBuf = getLogBuffer(getSelectedBufName());
  const displayLines = getDisplayLines(logBuf, logInner - 1);

  let buf = CURSOR_HIDE;
  const logCol = sidebarInner + 2; // after left border + sidebar + divider

  for (let r = mainTop; r <= mainBottom; r++) {
    const rowIdx = r - mainTop;
    buf += moveTo(r, logCol);
    buf += renderLogRow(rowIdx, logInner, displayLines, logBuf.scrollPos);
    buf += RESET;
  }

  buf += positionCmdCursor() + CURSOR_SHOW;
  process.stdout.write(buf);
}

function renderSidebar() {
  if (!layout) return;
  const { sidebarInner, mainTop, mainBottom } = layout;

  let buf = CURSOR_HIDE;
  for (let r = mainTop; r <= mainBottom; r++) {
    const rowIdx = r - mainTop;
    buf += moveTo(r, 1);
    buf += renderSidebarRow(rowIdx, sidebarInner);
    buf += RESET;
  }

  buf += positionCmdCursor() + CURSOR_SHOW;
  process.stdout.write(buf);
}

function renderCommandLine() {
  if (!layout) return;
  const { cols, cmdRow } = layout;

  let buf = CURSOR_HIDE;
  buf += moveTo(cmdRow, 1);
  buf += renderCmdContent(cols - 2);
  buf += positionCmdCursor() + CURSOR_SHOW;
  process.stdout.write(buf);
}

function renderCmdContent(width) {
  if (searchMode) {
    const count = searchMode.matches.length;
    const pos = searchMode.matchIdx >= 0 ? searchMode.matchIdx + 1 : 0;
    const countStr = count > 0 ? ` [${pos}/${count}]` : ' [no matches]';
    const content = `${BOLD}/${RESET}${searchMode.pattern}${DIM}${countStr}${RESET}`;
    return fitToWidth(content, width);
  }
  if (questionMode) {
    const content = questionMode.prompt + questionMode.input;
    return fitToWidth(content, width);
  }
  const style = focusArea === 'command' ? BOLD : DIM;
  const prompt = `${style}devctl>${RESET} `;
  return fitToWidth(prompt + cmdInput, width);
}

function positionCmdCursor() {
  if (!layout) return '';
  const { cmdRow } = layout;

  if (searchMode) {
    return moveTo(cmdRow, 1 + 1 + searchMode.pattern.length); // 1 = "/"
  }
  if (questionMode) {
    const promptLen = questionMode.prompt.length;
    return moveTo(cmdRow, 1 + promptLen + questionMode.cursor);
  }
  return moveTo(cmdRow, 1 + 8 + cmdCursor); // 8 = "devctl> "
}

// ── Search Mode Functions ───────────────────────────────

function enterSearchMode() {
  searchMode = {
    pattern: '',
    matches: [],
    matchIdx: -1,
    regex: null,
  };
  scheduleFullRender();
}

function exitSearchMode() {
  searchMode = null;
  scheduleFullRender();
}

function updateSearchMatches() {
  if (!searchMode || !searchMode.pattern) {
    searchMode.matches = [];
    searchMode.matchIdx = -1;
    searchMode.regex = null;
    return;
  }

  try {
    searchMode.regex = new RegExp(searchMode.pattern, 'gi');
  } catch {
    // Invalid regex, treat as literal
    const escaped = searchMode.pattern.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    searchMode.regex = new RegExp(escaped, 'gi');
  }

  const bufName = getSelectedBufName();
  const buf = getLogBuffer(bufName);
  const displayLines = getDisplayLines(buf, getLogTextWidth());

  searchMode.matches = [];
  for (let i = 0; i < displayLines.length; i++) {
    const line = stripAnsi(displayLines[i]);
    searchMode.regex.lastIndex = 0;
    let match;
    while ((match = searchMode.regex.exec(line)) !== null) {
      searchMode.matches.push({
        lineIdx: i,
        start: match.index,
        end: match.index + match[0].length,
      });
    }
  }

  // Update match index
  if (searchMode.matches.length > 0) {
    if (searchMode.matchIdx < 0) {
      searchMode.matchIdx = 0;
    } else if (searchMode.matchIdx >= searchMode.matches.length) {
      searchMode.matchIdx = searchMode.matches.length - 1;
    }
  } else {
    searchMode.matchIdx = -1;
  }
}

function navigateSearch(delta) {
  if (!searchMode || searchMode.matches.length === 0) return;

  searchMode.matchIdx += delta;
  if (searchMode.matchIdx < 0) {
    searchMode.matchIdx = searchMode.matches.length - 1;
  } else if (searchMode.matchIdx >= searchMode.matches.length) {
    searchMode.matchIdx = 0;
  }

  // Scroll to show the current match
  const match = searchMode.matches[searchMode.matchIdx];
  const bufName = getSelectedBufName();
  const buf = getLogBuffer(bufName);
  const viewHeight = getLogViewHeight();

  if (match.lineIdx < buf.scrollPos) {
    buf.scrollPos = match.lineIdx;
    buf.follow = false;
  } else if (match.lineIdx >= buf.scrollPos + viewHeight) {
    buf.scrollPos = match.lineIdx - viewHeight + 1;
    buf.follow = false;
  }

  renderLogPane();
  renderCommandLine();
}

function highlightSearchInLine(line, lineIdx) {
  if (!searchMode || !searchMode.regex || searchMode.matches.length === 0) {
    return line;
  }

  // Find matches on this line
  const lineMatches = searchMode.matches.filter(m => m.lineIdx === lineIdx);
  if (lineMatches.length === 0) return line;

  const stripped = stripAnsi(line);
  const HIGHLIGHT = '\x1b[43m\x1b[30m'; // Yellow background, black text
  const CURRENT = '\x1b[45m\x1b[37m';   // Magenta background, white text

  // Build highlighted string
  let result = '';
  let lastEnd = 0;

  // Sort matches by start position
  lineMatches.sort((a, b) => a.start - b.start);

  for (const m of lineMatches) {
    // Add text before match
    if (m.start > lastEnd) {
      result += stripped.slice(lastEnd, m.start);
    }
    // Add highlighted match
    const isCurrent = searchMode.matchIdx >= 0 &&
      searchMode.matches[searchMode.matchIdx].lineIdx === lineIdx &&
      searchMode.matches[searchMode.matchIdx].start === m.start;
    const hl = isCurrent ? CURRENT : HIGHLIGHT;
    result += hl + stripped.slice(m.start, m.end) + RESET;
    lastEnd = m.end;
  }

  // Add remaining text
  if (lastEnd < stripped.length) {
    result += stripped.slice(lastEnd);
  }

  return result;
}

// ── Hotkey Hints ────────────────────────────────────────

function getHints() {
  if (scanMode) {
    const n = scanMode.selected.size;
    const tabHint = scanMode.scanFocus === 'candidates' ? 'Tab: readme' : 'Tab: list';
    return `${DIM}Space: toggle │ a: all │ Enter: add (${n}) │ Esc: cancel │ ${tabHint} │ PgUp/Dn: scroll${RESET}`;
  }
  if (searchMode) {
    const count = searchMode.matches.length;
    const pos = searchMode.matchIdx >= 0 ? searchMode.matchIdx + 1 : 0;
    return `${DIM}Enter/Esc: exit │ n: next │ N: prev │ ${pos}/${count} matches${RESET}`;
  }
  if (questionMode) {
    return `${DIM}Enter: submit${RESET}`;
  }

  // Check for error notification
  if (errorNotification) {
    return `${RED}${errorNotification.message}${RESET}`;
  }

  // Check for errors in current app
  const selectedName = getSelectedBufName();
  const errorCount = selectedName !== SYSTEM_NAME ? getAppErrorCount(selectedName) : 0;
  const errorHint = errorCount > 0 ? ` │ e: copy error │ E: copy all` : '';

  if (focusArea === 'sidebar') {
    return `${DIM}Tab: command │ ↑↓/jk: nav │ s/S/r: start/stop/restart │ R: all │ PgUp/Dn: scroll${errorHint} │ ^C: quit${RESET}`;
  }
  return `${DIM}Tab: sidebar │ /: search │ ↑↓: history │ PgUp/Dn: scroll${errorHint} │ ^C: quit${RESET}`;
}

function renderBottomBar() {
  if (!layout) return;
  const { cols, bottomRow } = layout;

  const hints = getHints();
  const hintsVis = stripAnsi(hints);
  const fill = cols - 2 - 3 - hintsVis.length;

  let buf = CURSOR_HIDE;
  buf += moveTo(bottomRow, 0);
  if (fill >= 0) {
    buf += BOX.BL + BOX.H + ' ' + hints + ' ' + BOX.H.repeat(fill) + BOX.BR;
  } else {
    buf += BOX.BL + BOX.H.repeat(cols - 2) + BOX.BR;
  }
  buf += positionCmdCursor() + CURSOR_SHOW;
  process.stdout.write(buf);
}

// ── Helpers ─────────────────────────────────────────────

function appColor(name) {
  const idx = apps.findIndex(a => a.name === name);
  return COLORS[Math.abs(idx) % COLORS.length];
}

function formatUptime(startedAt) {
  if (!startedAt) return '-';
  const ms = Date.now() - startedAt;
  const secs = Math.floor(ms / 1000);
  const mins = Math.floor(secs / 60);
  const hrs  = Math.floor(mins / 60);
  if (hrs > 0)  return `${hrs}h ${mins % 60}m`;
  if (mins > 0) return `${mins}m ${secs % 60}s`;
  return `${secs}s`;
}

function askQuestion(prompt) {
  return new Promise(resolve => {
    questionMode = { prompt, resolve, input: '', cursor: 0 };
    renderCommandLine();
    renderBottomBar();
  });
}

function getApp(name) {
  return apps.find(a => a.name === name);
}

function getSelectedApp() {
  if (selectedIdx === 0) return null;
  return apps[selectedIdx - 1] || null;
}

function getSelectedBufName() {
  if (selectedIdx === 0) return SYSTEM_NAME;
  return apps[selectedIdx - 1]?.name || SYSTEM_NAME;
}

// ── Scan Helpers ────────────────────────────────────────

function walkForPackageJsons(baseDir, maxDepth = 5, _depth = 0) {
  const results = [];
  if (_depth > maxDepth) return results;

  let entries;
  try {
    entries = fs.readdirSync(baseDir, { withFileTypes: true });
  } catch {
    return results;
  }

  const pkgPath = path.join(baseDir, 'package.json');
  if (fs.existsSync(pkgPath)) {
    try {
      const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf-8'));
      if (pkg.scripts && pkg.scripts.dev) {
        results.push({ fullPath: baseDir, pkg });
      }
    } catch {}
  }

  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    if (SCAN_SKIP_DIRS.has(entry.name)) continue;
    results.push(...walkForPackageJsons(path.join(baseDir, entry.name), maxDepth, _depth + 1));
  }

  return results;
}

function isMonorepoRoot(fullPath) {
  return (
    fs.existsSync(path.join(fullPath, 'turbo.json')) ||
    fs.existsSync(path.join(fullPath, 'pnpm-workspace.yaml'))
  );
}

function isServerDevScript(devScript) {
  const serverPatterns = [
    /\bnext\b/, /\bvite\b/, /\bwrangler\b/, /\bexpo\b/,
    /\bnodemon\b/, /\btsx\s+watch\b/, /\bPORT=/, /--port\b/, /-p\s+\d+/,
  ];
  const excludePatterns = [
    /\btsc\b.*--watch/, /\btsup\b.*--watch/,
  ];
  if (excludePatterns.some(re => re.test(devScript))) return false;
  return serverPatterns.some(re => re.test(devScript));
}

function detectPackageManager(fullPath, pkg) {
  if (pkg.packageManager) {
    if (pkg.packageManager.startsWith('pnpm')) return 'pnpm';
    if (pkg.packageManager.startsWith('yarn')) return 'yarn';
    if (pkg.packageManager.startsWith('npm')) return 'npm';
  }

  let dir = fullPath;
  while (true) {
    if (fs.existsSync(path.join(dir, 'pnpm-lock.yaml'))) return 'pnpm';
    if (fs.existsSync(path.join(dir, 'yarn.lock'))) return 'yarn';
    if (fs.existsSync(path.join(dir, 'package-lock.json'))) return 'npm';
    const parent = path.dirname(dir);
    if (parent === dir || !dir.startsWith(PROJECT_ROOT)) break;
    dir = parent;
  }

  return 'npm';
}

function buildCommand(pm) {
  if (pm === 'pnpm') return 'pnpm dev';
  if (pm === 'yarn') return 'yarn dev';
  return 'npm run dev';
}

function detectPorts(devScript, fullPath) {
  // 1. PORT=(\d+) in dev script
  let m = devScript.match(/PORT=(\d+)/);
  if (m) return [parseInt(m[1], 10)];

  // 2. -p (\d+) or --port (\d+) in dev script
  m = devScript.match(/(?:-p\s+|--port\s+)(\d+)/);
  if (m) return [parseInt(m[1], 10)];

  // 3. port in vite.config.{ts,js,mjs}
  for (const ext of ['ts', 'js', 'mjs']) {
    const vitePath = path.join(fullPath, `vite.config.${ext}`);
    if (fs.existsSync(vitePath)) {
      try {
        const content = fs.readFileSync(vitePath, 'utf-8');
        const pm = content.match(/port\s*:\s*(\d+)/);
        if (pm) return [parseInt(pm[1], 10)];
      } catch {}
    }
  }

  // 4. port in wrangler.toml
  const wranglerPath = path.join(fullPath, 'wrangler.toml');
  if (fs.existsSync(wranglerPath)) {
    try {
      const content = fs.readFileSync(wranglerPath, 'utf-8');
      const pm = content.match(/port\s*=\s*(\d+)/);
      if (pm) return [parseInt(pm[1], 10)];
    } catch {}
  }

  // 5. Framework defaults
  if (/\bnext\b/.test(devScript)) return [3000];
  if (/\bvite\b/.test(devScript)) return [5173];
  if (/\bwrangler\b/.test(devScript)) return [8787];
  if (/\bexpo\b/.test(devScript)) return [8081];

  // 6. Unknown
  return [];
}

function extractName(pkg, relDir) {
  if (pkg.name) {
    return pkg.name.replace(/^@[^/]+\//, '');
  }
  return path.basename(relDir);
}

function detectApps() {
  const found = walkForPackageJsons(PROJECT_ROOT);

  // Identify monorepo roots that have child results → exclude them
  const monorepoRoots = new Set();
  for (const { fullPath } of found) {
    if (isMonorepoRoot(fullPath)) {
      const hasChild = found.some(
        other => other.fullPath !== fullPath && other.fullPath.startsWith(fullPath + path.sep),
      );
      if (hasChild) monorepoRoots.add(fullPath);
    }
  }

  const registeredDirs = new Set(apps.map(a => path.resolve(PROJECT_ROOT, a.dir)));
  const candidates = [];

  for (const { fullPath, pkg } of found) {
    if (monorepoRoots.has(fullPath)) continue;

    const devScript = pkg.scripts.dev;
    if (!isServerDevScript(devScript)) continue;
    if (registeredDirs.has(fullPath)) continue;

    const ports = detectPorts(devScript, fullPath);
    if (ports.length === 0) continue;

    const relDir = path.relative(PROJECT_ROOT, fullPath);
    const name = extractName(pkg, relDir);
    const pm = detectPackageManager(fullPath, pkg);
    const command = buildCommand(pm);

    candidates.push({ name, dir: relDir, command, ports, devScript });
  }

  return candidates;
}

function parseSelection(input, max) {
  const trimmed = input.trim().toLowerCase();
  if (trimmed === 'all') {
    return Array.from({ length: max }, (_, i) => i);
  }
  const indices = new Set();
  for (const token of trimmed.split(',')) {
    const num = parseInt(token.trim(), 10);
    if (!isNaN(num) && num >= 1 && num <= max) indices.add(num - 1);
  }
  return [...indices].sort((a, b) => a - b);
}

// ── Process Manager ─────────────────────────────────────

async function startApp(name) {
  const app = getApp(name);
  if (!app) {
    log(`${RED}Unknown app: ${name}${RESET}`);
    return;
  }

  const existing = procs.get(name);
  if (existing && existing.status === 'running') {
    appendLog(name, `${YELLOW}${name} is already running (PID ${existing.proc.pid}). Use 'restart' instead.${RESET}`);
    return;
  }
  if (existing && existing.status === 'stopping') {
    appendLog(name, `${YELLOW}${name} is still stopping. Please wait.${RESET}`);
    return;
  }

  const fullDir = path.resolve(PROJECT_ROOT, app.dir);
  if (!fs.existsSync(fullDir)) {
    appendLog(name, `${YELLOW}Warning: directory does not exist: ${fullDir}${RESET}`);
  }

  // Port conflict check with resolution options
  const portResults = await Promise.all(
    app.ports.map(async p => ({ port: p, free: await isPortFree(p) })),
  );
  const taken = portResults.filter(r => !r.free);

  if (taken.length > 0) {
    // Check each taken port for owner info
    for (const { port } of taken) {
      const ownerInfo = await getPortOwnerInfo(port);

      if (!ownerInfo) {
        // Couldn't identify owner, fall back to simple prompt
        appendLog(name, `${YELLOW}⚠ Port ${port} is in use by unknown process${RESET}`);
        const answer = await askQuestion(`Start ${name} anyway? (y/N): `);
        if (answer.toLowerCase() !== 'y') {
          appendLog(name, 'Start cancelled.');
          return;
        }
        continue;
      }

      // Check if it's a devctl-managed app
      const devctlApp = findDevctlOwner(ownerInfo.pid);

      if (devctlApp) {
        // Port is used by another devctl-managed app
        appendLog(name, `${YELLOW}⚠ Port ${port} is used by devctl app "${devctlApp}" (running)${RESET}`);
        appendLog(name, '');

        // Get alternative port suggestion
        const suggestions = await suggestAlternativePorts([port]);
        const altPort = suggestions[0]?.suggested;

        appendLog(name, 'Options:');
        appendLog(name, `  ${BOLD}[r]${RESET} Restart ${devctlApp}, then start this app`);
        if (altPort) {
          appendLog(name, `  ${BOLD}[a]${RESET} Use alternative port (${altPort} is free)`);
        }
        appendLog(name, `  ${BOLD}[s]${RESET} Start anyway (may fail)`);
        appendLog(name, `  ${BOLD}[c]${RESET} Cancel`);
        appendLog(name, '');

        const validChoices = altPort ? ['r', 'a', 's', 'c'] : ['r', 's', 'c'];
        let choice = '';
        while (!validChoices.includes(choice.toLowerCase())) {
          choice = await askQuestion(`Choice (${validChoices.join('/')}): `);
          if (!validChoices.includes(choice.toLowerCase())) {
            appendLog(name, `${DIM}Invalid choice. Please enter ${validChoices.join(', ')}.${RESET}`);
          }
        }

        switch (choice.toLowerCase()) {
          case 'r':
            appendLog(name, `Restarting ${devctlApp}...`);
            await restartApp(devctlApp);
            // Re-check if port is now free
            if (!await isPortFree(port)) {
              appendLog(name, `${RED}Port ${port} still in use after restart. Start cancelled.${RESET}`);
              return;
            }
            break;
          case 'a':
            appendLog(name, `${DIM}Note: Using alternative port ${altPort}. Update your app config if needed.${RESET}`);
            // Continue with start - the app will need to use the alternative port
            break;
          case 's':
            appendLog(name, `${DIM}Starting anyway...${RESET}`);
            break;
          case 'c':
            appendLog(name, 'Start cancelled.');
            return;
        }
      } else {
        // External process
        appendLog(name, `${YELLOW}⚠ Port ${port} is in use by external process:${RESET}`);
        appendLog(name, `  PID: ${ownerInfo.pid}, Command: ${ownerInfo.command}, User: ${ownerInfo.user}`);
        appendLog(name, '');

        // Get alternative port suggestion
        const suggestions = await suggestAlternativePorts([port]);
        const altPort = suggestions[0]?.suggested;

        appendLog(name, 'Options:');
        appendLog(name, `  ${BOLD}[k]${RESET} Kill the process and start`);
        if (altPort) {
          appendLog(name, `  ${BOLD}[a]${RESET} Use alternative port (${altPort} is free)`);
        }
        appendLog(name, `  ${BOLD}[s]${RESET} Start anyway (may fail)`);
        appendLog(name, `  ${BOLD}[c]${RESET} Cancel`);
        appendLog(name, '');

        const validChoices = altPort ? ['k', 'a', 's', 'c'] : ['k', 's', 'c'];
        let choice = '';
        while (!validChoices.includes(choice.toLowerCase())) {
          choice = await askQuestion(`Choice (${validChoices.join('/')}): `);
          if (!validChoices.includes(choice.toLowerCase())) {
            appendLog(name, `${DIM}Invalid choice. Please enter ${validChoices.join(', ')}.${RESET}`);
          }
        }

        switch (choice.toLowerCase()) {
          case 'k':
            appendLog(name, `Killing process ${ownerInfo.pid}...`);
            const killResult = await killExternalProcess(ownerInfo.pid);
            if (!killResult.success) {
              if (killResult.reason === 'permission') {
                appendLog(name, `${RED}Permission denied. Try running with sudo or kill manually.${RESET}`);
              } else {
                appendLog(name, `${RED}Failed to kill process.${RESET}`);
              }
              appendLog(name, 'Start cancelled.');
              return;
            }
            // Verify port is now free
            await new Promise(r => setTimeout(r, 500));
            if (!await isPortFree(port)) {
              appendLog(name, `${RED}Port ${port} still in use. Start cancelled.${RESET}`);
              return;
            }
            appendLog(name, `${GREEN}Process killed successfully.${RESET}`);
            break;
          case 'a':
            appendLog(name, `${DIM}Note: Using alternative port ${altPort}. Update your app config if needed.${RESET}`);
            break;
          case 's':
            appendLog(name, `${DIM}Starting anyway...${RESET}`);
            break;
          case 'c':
            appendLog(name, 'Start cancelled.');
            return;
        }
      }
    }
  }

  const proc = spawn(app.command, [], {
    shell: true,
    cwd: fullDir,
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: true,
    env: { ...process.env, TURBO_UI: 'stream' },
  });

  const existingEntry = procs.get(name);
  const entry = {
    proc,
    status: 'running',
    startedAt: Date.now(),
    exitCode: null,
    killTimer: null,
    stopResolve: null,
    restartCount: existingEntry?.restartCount || 0,
    autoRestartDisabled: existingEntry?.autoRestartDisabled || false,
  };
  procs.set(name, entry);

  proc.stdout.on('data', data => appendLog(name, data));
  proc.stderr.on('data', data => appendLog(name, data, true));

  proc.on('error', err => {
    entry.status = 'crashed';
    appendLog(name, `${RED}[${name}] failed to start: ${err.message}${RESET}`);
    scheduleFullRender();
  });

  proc.on('exit', (code, signal) => {
    const wasStopping = entry.status === 'stopping';

    if (wasStopping) {
      entry.status = 'stopped';
      entry.restartCount = 0; // Reset restart count on manual stop
      appendLog(name, `${DIM}Stopped ${name}.${RESET}`);
    } else {
      entry.status = 'crashed';
      entry.exitCode = code;
      appendLog(name, `${RED}[${name}] exited (code=${code}, signal=${signal})${RESET}`);
    }

    entry.proc = null;
    entry.startedAt = null;

    if (entry.killTimer) {
      clearTimeout(entry.killTimer);
      entry.killTimer = null;
    }
    if (entry.stopResolve) {
      entry.stopResolve();
      entry.stopResolve = null;
    }

    scheduleFullRender();

    // Auto-restart logic (only for crashes, not manual stops)
    if (!wasStopping && !shuttingDown && !entry.autoRestartDisabled) {
      const appConfig = getApp(name);
      if (appConfig?.autoRestart) {
        const maxRestarts = appConfig.maxRestarts ?? 5;
        const restartDelay = appConfig.restartDelay ?? 3000;

        if (entry.restartCount < maxRestarts) {
          entry.restartCount++;
          appendLog(name, `${YELLOW}Auto-restarting in ${restartDelay}ms (attempt ${entry.restartCount}/${maxRestarts})...${RESET}`);
          setTimeout(() => {
            if (!shuttingDown && entry.status === 'crashed') {
              startApp(name).catch(e => {
                appendLog(name, `${RED}Auto-restart failed: ${e.message}${RESET}`);
              });
            }
          }, restartDelay);
        } else {
          appendLog(name, `${RED}Auto-restart limit reached (${maxRestarts} attempts). Use 'start ${name}' to restart manually.${RESET}`);
        }
      }
    }
  });

  appendLog(name, `${GREEN}Started ${name}${RESET} (PID ${proc.pid})`);
  scheduleFullRender();
}

async function stopApp(name) {
  const entry = procs.get(name);
  if (!entry || entry.status !== 'running') {
    appendLog(name, `${DIM}${name} is not running.${RESET}`);
    return;
  }

  entry.status = 'stopping';
  const pid = entry.proc.pid;
  scheduleFullRender();

  return new Promise(resolve => {
    entry.stopResolve = resolve;

    entry.killTimer = setTimeout(() => {
      appendLog(name, `${YELLOW}[${name}] SIGTERM timeout, sending SIGKILL...${RESET}`);
      try { process.kill(-pid, 'SIGKILL'); } catch {}
    }, 5000);

    try {
      process.kill(-pid, 'SIGTERM');
    } catch {
      clearTimeout(entry.killTimer);
      entry.killTimer = null;
      entry.status = 'stopped';
      entry.proc = null;
      entry.startedAt = null;
      entry.stopResolve = null;
      scheduleFullRender();
      resolve();
    }
  });
}

async function restartApp(name) {
  await stopApp(name);
  await startApp(name);
}

async function stopAll() {
  const running = [...procs.entries()].filter(([, e]) => e.status === 'running');
  if (running.length === 0) return;
  log(`Stopping ${running.length} app(s)...`);
  await Promise.allSettled(running.map(([n]) => stopApp(n)));
}

// ── Commands ────────────────────────────────────────────

async function cmdStart(args) {
  if (!args) { log('Usage: start <name|all>'); return; }
  if (args === 'all') {
    // Filter to apps that aren't already running
    const appsToStart = apps.filter(app => {
      const entry = procs.get(app.name);
      return !entry || entry.status !== 'running';
    });

    if (appsToStart.length === 0) {
      log('All apps are already running.');
      return;
    }

    // Pre-check port conflicts for parallel start optimization
    log(`${DIM}Checking ports for ${appsToStart.length} app(s)...${RESET}`);
    const { conflictFree, conflicting } = await checkPortConflicts(appsToStart);

    // Start conflict-free apps in parallel
    if (conflictFree.length > 0) {
      log(`Starting ${conflictFree.length} app(s) in parallel...`);
      await Promise.allSettled(conflictFree.map(app => startApp(app.name)));
    }

    // Start conflicting apps sequentially (they need interactive prompts)
    if (conflicting.length > 0) {
      log(`${YELLOW}${conflicting.length} app(s) have port conflicts, starting sequentially...${RESET}`);
      for (const app of conflicting) {
        await startApp(app.name);
      }
    }

    // Log summary of app statuses
    log('');
    log(`${BOLD}App Status Summary${RESET}`);
    for (const app of apps) {
      const entry = procs.get(app.name);
      const status = entry?.status || 'stopped';
      const statusColor = status === 'running' ? GREEN
        : status === 'crashed' ? RED
        : status === 'stopping' ? YELLOW
        : DIM;
      const dot = status === 'running' || status === 'crashed' || status === 'stopping' ? '\u25cf' : '\u25cb';
      log(`  ${statusColor}${dot}${RESET} ${app.name}: ${statusColor}${status}${RESET}`);
    }
    log('');
  } else {
    await startApp(args);
  }
}

async function cmdStop(args) {
  if (!args) { log('Usage: stop <name|all>'); return; }
  if (args === 'all') {
    await stopAll();
  } else {
    await stopApp(args);
  }
}

async function cmdRestart(args) {
  if (!args) { log('Usage: restart <name|all>'); return; }
  if (args === 'all') {
    for (const app of apps) await restartApp(app.name);
  } else {
    await restartApp(args);
  }
}

function cmdStatus(args) {
  const list = args ? apps.filter(a => a.name === args) : apps;
  if (args && list.length === 0) { log(`${RED}Unknown app: ${args}${RESET}`); return; }
  if (list.length === 0) { log('No apps configured.'); return; }

  const headers = ['NAME', 'STATUS', 'PID', 'UPTIME', 'PORTS'];
  const rows = list.map(app => {
    const entry = procs.get(app.name);
    const status = entry?.status || 'stopped';
    const pid = (status === 'running' && entry?.proc?.pid) ? String(entry.proc.pid) : '-';
    const uptime = status === 'running' ? formatUptime(entry?.startedAt) : '-';
    return [app.name, status, pid, uptime, app.ports.join(', ')];
  });

  const widths = headers.map((h, i) =>
    Math.max(h.length, ...rows.map(r => r[i].length)),
  );

  log(BOLD + headers.map((h, i) => h.padEnd(widths[i])).join('  ') + RESET);
  for (const row of rows) {
    const statusColor =
      { running: GREEN, crashed: RED, stopping: YELLOW }[row[1]] || DIM;
    const cells = row.map((cell, i) =>
      i === 1
        ? `${statusColor}${cell.padEnd(widths[i])}${RESET}`
        : cell.padEnd(widths[i]),
    );
    log(cells.join('  '));
  }
}

async function cmdPorts() {
  if (apps.length === 0) { log('No apps configured.'); return; }

  const allPorts = [...new Set(apps.flatMap(a => a.ports))];
  const portInfo = {};

  // Check port status and get owner info for taken ports
  await Promise.all(allPorts.map(async p => {
    const free = await isPortFree(p);
    if (free) {
      portInfo[p] = { free: true, owner: null };
    } else {
      const owner = await getPortOwnerInfo(p);
      portInfo[p] = { free: false, owner };
    }
  }));

  log(`${BOLD}${'PORT'.padEnd(8)}${'STATUS'.padEnd(10)}${'APP'.padEnd(20)}OWNER${RESET}`);
  for (const app of apps) {
    for (const port of app.ports) {
      const info = portInfo[port];
      const color = info.free ? GREEN : RED;
      const label = info.free ? 'free' : 'in use';

      let ownerStr = '';
      if (!info.free && info.owner) {
        const devctlApp = findDevctlOwner(info.owner.pid);
        if (devctlApp) {
          ownerStr = `${DIM}devctl:${devctlApp}${RESET}`;
        } else {
          ownerStr = `${DIM}${info.owner.command} (PID ${info.owner.pid})${RESET}`;
        }
      } else if (!info.free) {
        ownerStr = `${DIM}unknown${RESET}`;
      }

      log(`${String(port).padEnd(8)}${color}${label.padEnd(10)}${RESET}${app.name.padEnd(20)}${ownerStr}`);
    }
  }
}

async function cmdScan() {
  const candidates = detectApps();

  if (candidates.length === 0) {
    log('No new apps detected.');
    return;
  }

  scanMode = {
    candidates,
    cursorIdx: 0,
    selected: new Set(),
    readmeCache: new Map(),
    readmeScrollPos: 0,
    candidateScroll: 0,
    scanFocus: 'candidates',
  };

  layout = calcLayout();
  scheduleFullRender();
}

function exitScanMode(confirmed) {
  if (confirmed && scanMode.selected.size > 0) {
    const addedNames = [];
    const existingNames = new Set(apps.map(a => a.name));

    for (const idx of scanMode.selected) {
      const c = scanMode.candidates[idx];
      let name = c.name;

      // Auto-resolve name collisions with numeric suffix
      if (existingNames.has(name)) {
        let suffix = 2;
        while (existingNames.has(`${c.name}-${suffix}`)) suffix++;
        name = `${c.name}-${suffix}`;
      }

      const entry = { name, dir: c.dir, command: c.command, ports: c.ports };
      const err = validateAppEntry(entry);
      if (err) continue;

      apps.push(entry);
      existingNames.add(name);
      addedNames.push(name);
    }

    scanMode = null;
    layout = calcLayout();
    scheduleFullRender();

    if (addedNames.length > 0) {
      saveConfig(apps);
      log(`${GREEN}Added ${addedNames.length} app(s): ${addedNames.join(', ')}${RESET}`);
    } else {
      log('No apps were added.');
    }
  } else {
    scanMode = null;
    layout = calcLayout();
    scheduleFullRender();
    log(confirmed ? 'No apps selected.' : 'Scan cancelled.');
  }
}

async function cmdAdd() {
  const name = await askQuestion('App name: ');
  if (!name) { log('Cancelled.'); return; }
  if (getApp(name)) { log(`${RED}App "${name}" already exists.${RESET}`); return; }

  const dir = await askQuestion('Directory (relative to project root): ');
  if (!dir) { log('Cancelled.'); return; }

  const command = await askQuestion('Command: ');
  if (!command) { log('Cancelled.'); return; }

  const portsStr = await askQuestion('Ports (comma-separated): ');
  const ports = portsStr
    .split(',')
    .map(s => parseInt(s.trim(), 10))
    .filter(n => !isNaN(n) && n > 0 && n < 65536);
  if (ports.length === 0) { log(`${RED}No valid ports provided.${RESET}`); return; }

  const entry = { name, dir, command, ports };
  const err = validateAppEntry(entry);
  if (err) { log(`${RED}Invalid entry: ${err}${RESET}`); return; }

  apps.push(entry);
  saveConfig(apps);
  log(`${GREEN}Added ${name}.${RESET}`);

  layout = calcLayout();
  scheduleFullRender();
}

async function cmdRemove(args) {
  if (!args) { log('Usage: remove <name>'); return; }
  const app = getApp(args);
  if (!app) { log(`${RED}Unknown app: ${args}${RESET}`); return; }

  const entry = procs.get(args);
  if (entry && entry.status === 'running') {
    const answer = await askQuestion(`${args} is running. Stop it first? (y/N): `);
    if (answer.toLowerCase() !== 'y') { log('Remove cancelled.'); return; }
    await stopApp(args);
  }

  apps = apps.filter(a => a.name !== args);
  procs.delete(args);
  logBuffers.delete(args);
  saveConfig(apps);
  log(`${GREEN}Removed ${args}.${RESET}`);

  if (selectedIdx > apps.length) {
    selectedIdx = apps.length;
  }
  layout = calcLayout();
  scheduleFullRender();
}

function cmdList() {
  if (apps.length === 0) { log('No apps configured.'); return; }
  for (const app of apps) {
    log(`${BOLD}${app.name}${RESET}`);
    log(`  dir:     ${app.dir}`);
    log(`  command: ${app.command}`);
    log(`  ports:   ${app.ports.join(', ')}`);
  }
}

async function cmdAutoRestart(args) {
  if (!args) {
    // Show current auto-restart status for all apps
    log(`${BOLD}Auto-Restart Status${RESET}`);
    for (const app of apps) {
      const entry = procs.get(app.name);
      const configEnabled = app.autoRestart ?? false;
      const runtimeDisabled = entry?.autoRestartDisabled ?? false;
      const effective = configEnabled && !runtimeDisabled;
      const statusColor = effective ? GREEN : DIM;
      const statusText = configEnabled
        ? (runtimeDisabled ? 'disabled (runtime)' : 'enabled')
        : 'disabled (config)';
      const restartInfo = entry?.restartCount ? ` [${entry.restartCount} restarts]` : '';
      log(`  ${statusColor}${app.name}${RESET}: ${statusText}${restartInfo}`);
    }
    log('');
    log(`${DIM}Usage: autorestart <name> [on|off]${RESET}`);
    return;
  }

  const parts = args.split(/\s+/);
  const name = parts[0];
  const action = parts[1]?.toLowerCase();

  const app = getApp(name);
  if (!app) {
    log(`${RED}Unknown app: ${name}${RESET}`);
    return;
  }

  if (!action) {
    // Toggle
    const entry = procs.get(name);
    if (entry) {
      entry.autoRestartDisabled = !entry.autoRestartDisabled;
      const status = entry.autoRestartDisabled ? 'disabled' : 'enabled';
      log(`Auto-restart for ${name}: ${status} (runtime)`);
    } else {
      log(`${name} has not been started yet. Auto-restart config: ${app.autoRestart ? 'enabled' : 'disabled'}`);
    }
    return;
  }

  if (action === 'on') {
    const entry = procs.get(name);
    if (entry) {
      entry.autoRestartDisabled = false;
      entry.restartCount = 0;
    }
    log(`Auto-restart for ${name}: enabled (runtime)`);
  } else if (action === 'off') {
    const entry = procs.get(name);
    if (entry) {
      entry.autoRestartDisabled = true;
    }
    log(`Auto-restart for ${name}: disabled (runtime)`);
  } else {
    log(`${RED}Invalid action: ${action}. Use 'on' or 'off'.${RESET}`);
  }
}

function cmdClearErrors(args) {
  if (!args) {
    // Clear errors for currently selected app
    const selectedName = getSelectedBufName();
    if (selectedName === SYSTEM_NAME) {
      log(`${DIM}No app selected. Use 'clear-errors <name>' or 'clear-errors all'${RESET}`);
      return;
    }
    clearErrors(selectedName);
    log(`Errors cleared for ${selectedName}`);
    scheduleFullRender();
    return;
  }

  if (args === 'all') {
    clearErrors('all');
    log('All errors cleared');
    scheduleFullRender();
    return;
  }

  const app = getApp(args);
  if (!app) {
    log(`${RED}Unknown app: ${args}${RESET}`);
    return;
  }

  clearErrors(args);
  log(`Errors cleared for ${args}`);
  scheduleFullRender();
}

function hasAppChanged(oldApp, newApp) {
  if (oldApp.dir !== newApp.dir) return true;
  if (oldApp.command !== newApp.command) return true;
  if (oldApp.ports.length !== newApp.ports.length) return true;
  if (!oldApp.ports.every((p, i) => p === newApp.ports[i])) return true;
  return false;
}

function describeChanges(oldApp, newApp) {
  const changes = [];
  if (oldApp.dir !== newApp.dir) {
    changes.push(`dir: ${oldApp.dir} → ${newApp.dir}`);
  }
  if (oldApp.command !== newApp.command) {
    changes.push(`command: ${oldApp.command} → ${newApp.command}`);
  }
  if (oldApp.ports.join(',') !== newApp.ports.join(',')) {
    changes.push(`ports: ${oldApp.ports.join(',')} → ${newApp.ports.join(',')}`);
  }
  return changes;
}

async function cmdReload() {
  log('Reloading config from apps.json...');

  const newApps = loadConfig();
  const oldAppMap = new Map(apps.map(a => [a.name, a]));
  const newAppMap = new Map(newApps.map(a => [a.name, a]));

  const added = [];
  const removed = [];
  const changed = [];

  // Find added and changed apps
  for (const [name, newApp] of newAppMap) {
    const oldApp = oldAppMap.get(name);
    if (!oldApp) {
      added.push(newApp);
    } else if (hasAppChanged(oldApp, newApp)) {
      changed.push({ name, oldApp, newApp });
    }
  }

  // Find removed apps
  for (const [name] of oldAppMap) {
    if (!newAppMap.has(name)) {
      removed.push(name);
    }
  }

  if (added.length === 0 && removed.length === 0 && changed.length === 0) {
    log('No changes detected.');
    return;
  }

  // Report what was found
  if (added.length > 0) {
    log(`${GREEN}Added: ${added.map(a => a.name).join(', ')}${RESET}`);
  }
  if (removed.length > 0) {
    log(`${RED}Removed: ${removed.join(', ')}${RESET}`);
  }
  if (changed.length > 0) {
    log(`${YELLOW}Changed: ${changed.map(c => c.name).join(', ')}${RESET}`);
    for (const { name, oldApp, newApp } of changed) {
      const desc = describeChanges(oldApp, newApp);
      for (const d of desc) {
        log(`  ${DIM}${name}: ${d}${RESET}`);
      }
    }
  }

  // Handle removed apps
  for (const name of removed) {
    const entry = procs.get(name);
    if (entry && entry.status === 'running') {
      const answer = await askQuestion(`${name} was removed but is running. Stop it? (y/N): `);
      if (answer.toLowerCase() === 'y') {
        await stopApp(name);
      }
    }
    procs.delete(name);
    logBuffers.delete(name);
  }

  // Handle changed apps
  for (const { name } of changed) {
    const entry = procs.get(name);
    if (entry && entry.status === 'running') {
      const answer = await askQuestion(`${name} config changed. Restart it? (y/N): `);
      if (answer.toLowerCase() === 'y') {
        await stopApp(name);
        // Will restart with new config after apps array is updated
      }
    }
  }

  // Update apps array
  apps = newApps;

  // Restart changed apps that were stopped
  for (const { name } of changed) {
    const entry = procs.get(name);
    if (entry && entry.status === 'stopped') {
      await startApp(name);
    }
  }

  // Fix selected index if needed
  if (selectedIdx > apps.length) {
    selectedIdx = apps.length;
  }

  layout = calcLayout();
  scheduleFullRender();
  log(`${GREEN}Config reloaded successfully.${RESET}`);
}

function cmdHelp() {
  log(`${BOLD}devctl${RESET} \u2014 Multi-App Dev Server Manager`);
  log('');
  log(`  ${BOLD}start${RESET} <name|all>      Start an app (or all)`);
  log(`  ${BOLD}stop${RESET} <name|all>       Stop an app (or all)`);
  log(`  ${BOLD}restart${RESET} <name|all>    Restart an app (or all)`);
  log(`  ${BOLD}status${RESET} [name]         Show app status table`);
  log(`  ${BOLD}ports${RESET}                 Check port availability`);
  log(`  ${BOLD}scan${RESET}                  Auto-detect apps (batch select)`);
  log(`  ${BOLD}add${RESET}                   Add a new app interactively`);
  log(`  ${BOLD}remove${RESET} <name>         Remove an app from config`);
  log(`  ${BOLD}reload${RESET}                Reload config from apps.json`);
  log(`  ${BOLD}autorestart${RESET} [name]    View/toggle auto-restart`);
  log(`  ${BOLD}clear-errors${RESET} [name|all]  Clear detected errors`);
  log(`  ${BOLD}list${RESET}                  List configured apps`);
  log(`  ${BOLD}help${RESET}                  Show this help`);
  log(`  ${BOLD}quit${RESET}                  Stop all and exit`);
  log('');
  log(`${DIM}Tab: toggle sidebar/command  \u2191\u2193/j/k: navigate  PgUp/PgDn: scroll${RESET}`);
  log(`${DIM}e: copy last error  E: copy all errors  /: search logs${RESET}`);
}

async function cmdQuit() {
  await shutdown('Shutting down...');
}

// ── Completer ───────────────────────────────────────────

function completer(line) {
  if (questionMode) return [[], line];

  const parts = line.trimStart().split(/\s+/);
  const commands = [
    'start', 'stop', 'restart', 'status',
    'ports', 'scan', 'add', 'remove', 'reload', 'autorestart', 'clear-errors', 'list', 'help', 'quit',
  ];

  if (parts.length <= 1) {
    const partial = parts[0] || '';
    const hits = commands.filter(c => c.startsWith(partial));
    return [hits.length ? hits : commands, partial];
  }

  const cmd = parts[0];
  const partial = parts[1] || '';
  const withAll  = ['start', 'stop', 'restart', 'clear-errors'];
  const withName = ['start', 'stop', 'restart', 'status', 'remove', 'autorestart', 'clear-errors'];

  if (withName.includes(cmd)) {
    const names = apps.map(a => a.name);
    if (withAll.includes(cmd)) names.push('all');
    const hits = names.filter(n => n.startsWith(partial));
    return [hits.length ? hits : names, partial];
  }

  return [[], line];
}

// ── Command Dispatcher ──────────────────────────────────

async function handleCommand(line) {
  if (!line) return;
  const spaceIdx = line.indexOf(' ');
  const cmd  = spaceIdx === -1 ? line : line.slice(0, spaceIdx);
  const args = spaceIdx === -1 ? ''   : line.slice(spaceIdx + 1).trim();

  switch (cmd) {
    case 'start':   return cmdStart(args);
    case 'stop':    return cmdStop(args);
    case 'restart': return cmdRestart(args);
    case 'status':  return cmdStatus(args);
    case 'ports':   return cmdPorts();
    case 'scan':    return cmdScan();
    case 'add':     return cmdAdd();
    case 'remove':  return cmdRemove(args);
    case 'reload':  return cmdReload();
    case 'autorestart': return cmdAutoRestart(args);
    case 'clear-errors': return cmdClearErrors(args);
    case 'list':    return cmdList();
    case 'help':    return cmdHelp();
    case 'quit':
    case 'exit':    return cmdQuit();
    default:
      log(`Unknown command: ${cmd}. Type 'help' for available commands.`);
  }
}

// ── Input Handler ───────────────────────────────────────

function handleKeypress(str, key) {
  if (!key) return;

  // Ctrl+C → quit
  if (key.ctrl && key.name === 'c') {
    cmdQuit();
    return;
  }

  // Question mode
  if (questionMode) {
    handleQuestionKeypress(str, key);
    return;
  }

  // Scan mode
  if (scanMode) { handleScanKeypress(str, key); return; }

  // Search mode
  if (searchMode) { handleSearchKeypress(str, key); return; }

  // PageUp/PageDown in any focus mode
  if (key.name === 'pageup') {
    scrollLog(-(getLogViewHeight() - 1));
    return;
  }
  if (key.name === 'pagedown') {
    scrollLog(getLogViewHeight() - 1);
    return;
  }

  if (focusArea === 'sidebar') {
    handleSidebarKeypress(str, key);
  } else {
    handleCommandKeypress(str, key);
  }
}

function handleSidebarKeypress(str, key) {
  if (key.name === 'tab') {
    focusArea = 'command';
    renderSidebar();
    renderCommandLine();
    renderBottomBar();
    return;
  }

  if (key.name === 'up' || key.name === 'k') {
    if (selectedIdx > 0) {
      selectedIdx--;
      renderSidebar();
      renderLogPane();
      renderCommandLine();
    }
    return;
  }

  if (key.name === 'down' || key.name === 'j') {
    if (selectedIdx < apps.length) {
      selectedIdx++;
      renderSidebar();
      renderLogPane();
      renderCommandLine();
    }
    return;
  }

  // Enter in sidebar → switch to command line
  if (key.name === 'return') {
    focusArea = 'command';
    renderSidebar();
    renderCommandLine();
    renderBottomBar();
    return;
  }

  // Shift+R → restart all apps
  if (str === 'R') {
    cmdRestart('all');
    return;
  }

  // s → start selected app
  if (str === 's' && !key.ctrl && !key.meta) {
    const app = getSelectedApp();
    if (app) startApp(app.name);
    return;
  }

  // S → stop selected app
  if (str === 'S') {
    const app = getSelectedApp();
    if (app) stopApp(app.name);
    return;
  }

  // r → restart selected app
  if (str === 'r' && !key.ctrl && !key.meta) {
    const app = getSelectedApp();
    if (app) restartApp(app.name);
    return;
  }

  // e → copy last error to clipboard
  if (str === 'e' && !key.ctrl && !key.meta) {
    copyLastError();
    return;
  }

  // E → copy all errors to clipboard
  if (str === 'E' && !key.ctrl && !key.meta) {
    copyAllErrors();
    return;
  }

  // Any printable key → switch to command line and type it
  if (str && str.length === 1 && !key.ctrl && !key.meta) {
    focusArea = 'command';
    renderSidebar();
    renderBottomBar();
    handleCommandKeypress(str, key);
    return;
  }
}

async function copyLastError() {
  const selectedName = getSelectedBufName();
  if (selectedName === SYSTEM_NAME) {
    log(`${DIM}No app selected${RESET}`);
    return;
  }
  const text = getLastErrorText(selectedName);
  if (!text) {
    log(`${DIM}No errors to copy${RESET}`);
    return;
  }
  try {
    await copyToClipboard(text);
    log(`${GREEN}Last error copied to clipboard${RESET}`);
  } catch (err) {
    log(`${RED}Failed to copy: ${err.message}${RESET}`);
  }
}

async function copyAllErrors() {
  const selectedName = getSelectedBufName();
  if (selectedName === SYSTEM_NAME) {
    log(`${DIM}No app selected${RESET}`);
    return;
  }
  const text = getAllErrorsText(selectedName);
  if (!text) {
    log(`${DIM}No errors to copy${RESET}`);
    return;
  }
  try {
    await copyToClipboard(text);
    const count = getAppErrorCount(selectedName);
    log(`${GREEN}${count} error${count > 1 ? 's' : ''} copied to clipboard${RESET}`);
  } catch (err) {
    log(`${RED}Failed to copy: ${err.message}${RESET}`);
  }
}

function handleCommandKeypress(str, key) {
  // Tab: completion if there's text, toggle focus if empty
  if (key.name === 'tab') {
    if (cmdInput.length === 0) {
      focusArea = 'sidebar';
      renderSidebar();
      renderCommandLine();
      renderBottomBar();
    } else {
      handleTabCompletion();
    }
    return;
  }

  // Reset tab state on non-tab key
  tabState = null;

  // Enter: execute command
  if (key.name === 'return') {
    if (processing) return;
    executeCommand();
    return;
  }

  // History navigation
  if (key.name === 'up') {
    navigateHistory(-1);
    return;
  }
  if (key.name === 'down') {
    navigateHistory(1);
    return;
  }

  // Backspace
  if (key.name === 'backspace') {
    if (cmdCursor > 0) {
      cmdInput = cmdInput.slice(0, cmdCursor - 1) + cmdInput.slice(cmdCursor);
      cmdCursor--;
      renderCommandLine();
    }
    return;
  }

  // Delete
  if (key.name === 'delete') {
    if (cmdCursor < cmdInput.length) {
      cmdInput = cmdInput.slice(0, cmdCursor) + cmdInput.slice(cmdCursor + 1);
      renderCommandLine();
    }
    return;
  }

  // Left / Right
  if (key.name === 'left') {
    if (cmdCursor > 0) { cmdCursor--; renderCommandLine(); }
    return;
  }
  if (key.name === 'right') {
    if (cmdCursor < cmdInput.length) { cmdCursor++; renderCommandLine(); }
    return;
  }

  // Home / End
  if (key.name === 'home' || (key.ctrl && key.name === 'a')) {
    cmdCursor = 0; renderCommandLine(); return;
  }
  if (key.name === 'end' || (key.ctrl && key.name === 'e')) {
    cmdCursor = cmdInput.length; renderCommandLine(); return;
  }

  // Ctrl+U: clear line
  if (key.ctrl && key.name === 'u') {
    cmdInput = ''; cmdCursor = 0; renderCommandLine(); return;
  }

  // Ctrl+W: delete word
  if (key.ctrl && key.name === 'w') {
    if (cmdCursor > 0) {
      const before = cmdInput.slice(0, cmdCursor);
      const after = cmdInput.slice(cmdCursor);
      const trimmed = before.replace(/\S+\s*$/, '');
      cmdInput = trimmed + after;
      cmdCursor = trimmed.length;
      renderCommandLine();
    }
    return;
  }

  // / or Ctrl+F: enter search mode (only when command line is empty)
  if ((str === '/' || (key.ctrl && key.name === 'f')) && cmdInput.length === 0) {
    enterSearchMode();
    return;
  }

  // Regular character
  if (str && str.length === 1 && !key.ctrl && !key.meta) {
    cmdInput = cmdInput.slice(0, cmdCursor) + str + cmdInput.slice(cmdCursor);
    cmdCursor++;
    renderCommandLine();
  }
}

function handleSearchKeypress(str, key) {
  // Escape or Enter: exit search mode
  if (key.name === 'escape' || key.name === 'return') {
    exitSearchMode();
    return;
  }

  // n: next match
  if (str === 'n' && !key.ctrl && !key.meta) {
    navigateSearch(1);
    return;
  }

  // N: previous match
  if (str === 'N' && !key.ctrl && !key.meta) {
    navigateSearch(-1);
    return;
  }

  // Backspace: delete character
  if (key.name === 'backspace') {
    if (searchMode.pattern.length > 0) {
      searchMode.pattern = searchMode.pattern.slice(0, -1);
      updateSearchMatches();
      renderLogPane();
      renderCommandLine();
      renderBottomBar();
    }
    return;
  }

  // Ctrl+U: clear pattern
  if (key.ctrl && key.name === 'u') {
    searchMode.pattern = '';
    updateSearchMatches();
    renderLogPane();
    renderCommandLine();
    renderBottomBar();
    return;
  }

  // Regular character: add to pattern
  if (str && str.length === 1 && !key.ctrl && !key.meta) {
    searchMode.pattern += str;
    updateSearchMatches();
    // Jump to first match
    if (searchMode.matches.length > 0 && searchMode.matchIdx < 0) {
      searchMode.matchIdx = 0;
      navigateSearch(0); // This will scroll to the match
    }
    renderLogPane();
    renderCommandLine();
    renderBottomBar();
  }
}

function handleScanKeypress(str, key) {
  const { candidates, cursorIdx } = scanMode;

  // Tab: toggle focus between candidates and readme
  if (key.name === 'tab') {
    scanMode.scanFocus = scanMode.scanFocus === 'candidates' ? 'readme' : 'candidates';
    scheduleFullRender();
    return;
  }

  // Up / k
  if (key.name === 'up' || key.name === 'k') {
    if (scanMode.scanFocus === 'readme') {
      scrollScanReadme(-1);
      return;
    }
    if (cursorIdx > 0) {
      scanMode.cursorIdx--;
      scanMode.readmeScrollPos = 0;
      // Scroll viewport if cursor is above visible area
      if (scanMode.cursorIdx < scanMode.candidateScroll) {
        scanMode.candidateScroll = scanMode.cursorIdx;
      }
      scheduleFullRender();
    }
    return;
  }

  // Down / j
  if (key.name === 'down' || key.name === 'j') {
    if (scanMode.scanFocus === 'readme') {
      scrollScanReadme(1);
      return;
    }
    if (cursorIdx < candidates.length - 1) {
      scanMode.cursorIdx++;
      scanMode.readmeScrollPos = 0;
      // Scroll viewport if cursor is below visible area
      const visibleRows = layout ? layout.mainHeight - 1 : 10;
      if (scanMode.cursorIdx >= scanMode.candidateScroll + visibleRows) {
        scanMode.candidateScroll = scanMode.cursorIdx - visibleRows + 1;
      }
      scheduleFullRender();
    }
    return;
  }

  // Space: toggle selection
  if (key.name === 'space' || str === ' ') {
    if (scanMode.selected.has(cursorIdx)) {
      scanMode.selected.delete(cursorIdx);
    } else {
      scanMode.selected.add(cursorIdx);
    }
    scheduleFullRender();
    return;
  }

  // a: toggle all / none
  if (str === 'a') {
    if (scanMode.selected.size === candidates.length) {
      scanMode.selected.clear();
    } else {
      for (let i = 0; i < candidates.length; i++) {
        scanMode.selected.add(i);
      }
    }
    scheduleFullRender();
    return;
  }

  // Enter: confirm
  if (key.name === 'return') {
    exitScanMode(true);
    return;
  }

  // Escape: cancel
  if (key.name === 'escape') {
    exitScanMode(false);
    return;
  }

  // PgUp / PgDn: scroll readme
  if (key.name === 'pageup') {
    const viewHeight = layout ? layout.mainHeight - 5 : 10;
    scrollScanReadme(-(viewHeight - 1));
    return;
  }
  if (key.name === 'pagedown') {
    const viewHeight = layout ? layout.mainHeight - 5 : 10;
    scrollScanReadme(viewHeight - 1);
    return;
  }
}

function handleQuestionKeypress(str, key) {
  if (key.name === 'return') {
    const answer = questionMode.input.trim();
    const resolve = questionMode.resolve;
    questionMode = null;
    renderCommandLine();
    renderBottomBar();
    resolve(answer);
    return;
  }

  if (key.name === 'backspace') {
    if (questionMode.cursor > 0) {
      const q = questionMode;
      q.input = q.input.slice(0, q.cursor - 1) + q.input.slice(q.cursor);
      q.cursor--;
      renderCommandLine();
    }
    return;
  }

  if (key.name === 'left') {
    if (questionMode.cursor > 0) { questionMode.cursor--; renderCommandLine(); }
    return;
  }
  if (key.name === 'right') {
    if (questionMode.cursor < questionMode.input.length) {
      questionMode.cursor++; renderCommandLine();
    }
    return;
  }

  if (key.name === 'home') {
    questionMode.cursor = 0; renderCommandLine(); return;
  }
  if (key.name === 'end') {
    questionMode.cursor = questionMode.input.length; renderCommandLine(); return;
  }

  if (str && str.length === 1 && !key.ctrl && !key.meta) {
    const q = questionMode;
    q.input = q.input.slice(0, q.cursor) + str + q.input.slice(q.cursor);
    q.cursor++;
    renderCommandLine();
  }
}

// ── Command Execution ───────────────────────────────────

async function executeCommand() {
  const line = cmdInput.trim();
  if (!line) return;

  cmdHistory.push(line);
  if (cmdHistory.length > 100) cmdHistory.shift();
  historyIdx = -1;
  historyTemp = '';

  cmdInput = '';
  cmdCursor = 0;
  renderCommandLine();

  processing = true;
  try {
    await handleCommand(line);
  } catch (e) {
    log(`${RED}Error: ${e.message}${RESET}`);
  }
  processing = false;
  renderCommandLine();
}

// ── History Navigation ──────────────────────────────────

function navigateHistory(dir) {
  if (cmdHistory.length === 0) return;

  if (dir < 0) {
    if (historyIdx === -1) {
      historyTemp = cmdInput;
      historyIdx = cmdHistory.length - 1;
    } else if (historyIdx > 0) {
      historyIdx--;
    } else {
      return;
    }
    cmdInput = cmdHistory[historyIdx];
  } else {
    if (historyIdx === -1) return;
    if (historyIdx < cmdHistory.length - 1) {
      historyIdx++;
      cmdInput = cmdHistory[historyIdx];
    } else {
      historyIdx = -1;
      cmdInput = historyTemp;
    }
  }

  cmdCursor = cmdInput.length;
  renderCommandLine();
}

// ── Log Scrolling ───────────────────────────────────────

function scrollLog(delta) {
  const bufName = getSelectedBufName();
  const buf = getLogBuffer(bufName);
  const viewHeight = getLogViewHeight();
  const displayCount = getDisplayLineCount(buf, getLogTextWidth());
  const maxScroll = Math.max(0, displayCount - viewHeight);

  buf.scrollPos = Math.max(0, Math.min(maxScroll, buf.scrollPos + delta));
  buf.follow = buf.scrollPos >= maxScroll;

  renderLogPane();
  renderCommandLine();
}

// ── Tab Completion ──────────────────────────────────────

function handleTabCompletion() {
  if (!tabState) {
    const [matches, partial] = completer(cmdInput.slice(0, cmdCursor));
    if (matches.length === 0) return;

    if (matches.length === 1) {
      const completion = matches[0];
      const before = cmdInput.slice(0, cmdCursor - partial.length);
      const after = cmdInput.slice(cmdCursor);
      cmdInput = before + completion + ' ' + after;
      cmdCursor = before.length + completion.length + 1;
      renderCommandLine();
      return;
    }

    const common = commonPrefix(matches);
    if (common.length > partial.length) {
      const before = cmdInput.slice(0, cmdCursor - partial.length);
      const after = cmdInput.slice(cmdCursor);
      cmdInput = before + common + after;
      cmdCursor = before.length + common.length;
      renderCommandLine();
    }

    tabState = { matches, idx: 0, partial, origInput: cmdInput, origCursor: cmdCursor };
    log(`${DIM}Completions: ${matches.join('  ')}${RESET}`);
    return;
  }

  // Cycle through matches
  tabState.idx = (tabState.idx + 1) % tabState.matches.length;
  const match = tabState.matches[tabState.idx];
  const before = tabState.origInput.slice(0, tabState.origCursor - tabState.partial.length);
  const after = tabState.origInput.slice(tabState.origCursor);
  cmdInput = before + match + after;
  cmdCursor = before.length + match.length;
  renderCommandLine();
}

function commonPrefix(strs) {
  if (strs.length === 0) return '';
  let prefix = strs[0];
  for (let i = 1; i < strs.length; i++) {
    while (!strs[i].startsWith(prefix)) {
      prefix = prefix.slice(0, -1);
    }
  }
  return prefix;
}

// ── Terminal Setup / Cleanup ────────────────────────────

function setupTerminal() {
  process.stdout.write(ALT_SCREEN_ON + CLEAR_SCREEN + CURSOR_HIDE);
  process.stdin.setRawMode(true);
  process.stdin.resume();
  readline.emitKeypressEvents(process.stdin);
  process.stdin.on('keypress', handleKeypress);

  process.stdout.on('resize', () => {
    layout = calcLayout();
    scheduleFullRender();
  });
}

function cleanupTerminal() {
  if (terminalCleaned) return;
  terminalCleaned = true;
  tuiReady = false;
  if (renderTimer) { clearTimeout(renderTimer); renderTimer = null; }
  try { process.stdout.write(CURSOR_SHOW + ALT_SCREEN_OFF); } catch {}
  try { process.stdin.setRawMode(false); } catch {}
}

// ── Shutdown ────────────────────────────────────────────

async function shutdown(reason) {
  if (shuttingDown) return;
  shuttingDown = true;
  cleanupTerminal();
  closeConfigWatcher();
  console.log(reason);
  const forceExit = setTimeout(() => process.exit(1), 10000);
  forceExit.unref();
  await stopAll();
  process.exit(0);
}

// ── Main ────────────────────────────────────────────────

const startAllFlag = process.argv.includes('--start-all');

function main() {
  apps = loadConfig();
  // When --start-all, default to devctl system logs; otherwise first app
  selectedIdx = startAllFlag ? 0 : (apps.length > 0 ? 1 : 0);

  if (!process.stdout.isTTY) {
    console.error('devctl requires a TTY terminal.');
    process.exit(1);
  }

  setupTerminal();
  setupConfigWatcher();
  layout = calcLayout();
  tuiReady = true;

  renderFull();

  process.on('exit', cleanupTerminal);
  process.on('uncaughtException', (err) => {
    cleanupTerminal();
    console.error('Uncaught exception:', err);
    process.exit(1);
  });
  process.on('SIGINT', () => shutdown('Received SIGINT, shutting down...'));
  process.on('SIGTERM', () => shutdown('Received SIGTERM, shutting down...'));
  process.on('SIGHUP', () => shutdown('Received SIGHUP, shutting down...'));

  if (startAllFlag) {
    cmdStart('all').catch(e => {
      log(`${RED}Error: ${e.message}${RESET}`);
    });
  }
}

main();
