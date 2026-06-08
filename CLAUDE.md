# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
- Pre-configured sessions via YAML bundle files
- Native macOS `.app`/`.dmg` packaging via WKWebView (darwin only)

---

## Repository Structure

```
tui-streamer/
├── cmd/
│   ├── app/main.go          # macOS native WebView app entry point (darwin + CGO only)
│   └── server/main.go       # HTTP/WebSocket server entry point
├── internal/
│   ├── browser/open.go      # Cross-platform browser launcher
│   ├── bundle/bundle.go     # YAML bundle/BundleSet parser
│   ├── executor/executor.go # Command execution engine (streaming output)
│   ├── server/server.go     # HTTP routes and WebSocket upgrade handler
│   └── session/
│       ├── manager.go       # Thread-safe session registry
│       ├── session.go       # Session state + client broadcast
│       └── client.go        # WebSocket client read/write pumps
├── web/
│   ├── embed.go             # Go embed directive for static assets
│   └── static/
│       ├── index.html       # Web UI markup
│       ├── style.css        # Styling with 10 themes
│       └── app.js           # Vanilla JS frontend (no framework/build step)
├── build/darwin/
│   ├── Info.plist           # macOS app bundle metadata
│   └── entitlements.plist   # macOS code signing entitlements
├── examples/
│   ├── lorem-ipsum/         # Example: streaming external API output
│   └── network-bundle/      # Example: BundleSet with network diagnostics
├── .github/workflows/       # CI (lint/test/build) and release automation
├── scripts/
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # User-facing project documentation
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files that declare sessions to pre-create on startup. Supports two document kinds: `Bundle` (a named list of sessions) and `BundleSet` (an ordered collection of `Bundle` references). A single YAML file may contain multiple `---`-separated documents.

2. **Session** (`internal/session/session.go`) — Named execution context. Holds:
   - `ID`, `Name`, `CreatedAt` — identity and creation time
   - `PendingCommand` — optional command string pre-loaded from a bundle (shown in the command bar)
   - `BundleName` — name of the bundle this session belongs to (empty for ad-hoc sessions)
   - `Description` — optional Markdown string from a bundle entry, rendered in the web UI
   - `running`, `cancel` — process lifecycle state
   - `lineBuf` — rolling replay buffer (up to `maxLineBuf = 2000` lines) sent to late-joining WebSocket clients
   - `clients` — map of subscribed `*Client` connections

3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (`UUID → *Session`). Provides `Create`, `Get`, `List`, `Delete`. `List` returns snapshots sorted by creation time for stable ordering.

4. **Executor** (`internal/executor/executor.go`) — Spawns a process via `exec.CommandContext`, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs over a buffered channel (capacity 256). Each `Line` carries a `type` (`stdout`, `stderr`, `start`, `exit`, `error`), the text `data`, a Unix millisecond `timestamp`, and an optional `exit_code`. A `waitDelay` of 5 seconds prevents goroutine leaks when a killed process doesn't drain its pipes immediately.

5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s period, 60s deadline), and `sync.Once`-guarded cleanup. `Send()` is non-blocking and drops messages rather than blocking the broadcaster.

6. **Server** (`internal/server/server.go`) — HTTP mux. Created via `New(manager, cfg, staticFS)`. Routes:
   - `GET /` — serves embedded static files; injects `<title>` and `window.STARTUP_BUNDLE` into `index.html`
   - `GET /ws/{id}` — upgrades to WebSocket, creates a `Client`, registers it to the session
   - `GET /api/sessions` — list all sessions (`[]Info`)
   - `POST /api/sessions` — create session; body: `{"name":"string"}`
   - `GET /api/sessions/{id}` — get a single session snapshot
   - `DELETE /api/sessions/{id}` — kill and remove a session
   - `POST /api/sessions/{id}/exec` — execute a command (see exec request format below)
   - `POST /api/sessions/{id}/kill` — kill the running process
   - `POST /api/bundles` — parse a YAML bundle body and create sessions (max 4 MiB)

### Exec Request Format

`POST /api/sessions/{id}/exec` accepts:
```json
{
  "command": "ls -la",         // string OR ["ls", "-la"] array
  "dir": "/optional/cwd",      // overrides server default
  "env": ["KEY=VALUE"],        // additional env vars (optional)
  "stdout": true,              // override server-level stdout capture
  "stderr": true               // override server-level stderr capture
}
```
Returns `202 Accepted` with `{"status":"started"}`, or `409 Conflict` if a command is already running.

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, blink, standard 16 colours, 256-colour cube, and true-colour RGB for both foreground and background).
- **`MarkdownRenderer`** — Minimal safe Markdown-to-HTML converter used to render session `description` fields. Supports headings (h1–h3), paragraphs, unordered lists, fenced code blocks, and inline bold/italic/code/links. All text nodes are HTML-escaped before inline patterns are applied.
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints including bundle import.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect. Supports `wss://` when served over HTTPS.
- **`Terminal`** — Renders output lines with auto-scroll. Prunes oldest `terminal-line` nodes once the DOM exceeds `MAX_DOM_LINES = 2000` to prevent memory growth.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (`localStorage`), per-session output buffering (up to `MAX_BUFFER = 2000` lines per session), bundle import, and line selection.

#### Interactive Command Detection

The frontend maintains an `INTERACTIVE_COMMANDS` set of known curses/full-screen programs (`top`, `htop`, `vim`, `less`, `tmux`, etc.). When the user attempts to run one of these, the UI warns them that the command requires a real TTY and won't render usefully.

#### Line Selection and Copy

Click any terminal line to select it (highlighted); Shift-click to extend a range. A toolbar appears showing the count of selected lines with options to:
- **Copy** — copies selected lines as plain text to the clipboard
- **ray.so** — opens [ray.so](https://ray.so) with the selected text pre-filled for creating a shareable code image
- **Clear** — deselects all lines

#### Keyboard Shortcuts (Web UI)

| Shortcut | Action |
|---|---|
| `Ctrl/⌘ + U` | Open "New Session" modal |
| `Ctrl/⌘ + O` | Open bundle file picker (when no startup bundle) |
| `Ctrl/⌘ + Shift + C` | Copy all terminal output |

#### Frontend Globals

`window.STARTUP_BUNDLE` (boolean) is injected by the server into `index.html` at request time. When `true`, the Import button is hidden and the New Session button is expanded to full width, since the session list is fully managed by the pre-loaded bundle.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: validate, whitelist-check, call session.Exec()
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: append to lineBuf, broadcast Line JSON to all clients
                       ↓
               client.go: write to WebSocket send channel (non-blocking)
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders
```

Late-joining clients receive the buffered output (up to `maxLineBuf = 2000` lines) replayed from `session.lineBuf` during the `subscribe()` call, under the session mutex to prevent races with concurrent broadcasts.

---

## Development Workflows

### Prerequisites

- Go 1.22+
- `make`
- macOS with Xcode command-line tools (only for the native app target)

### Common Commands

```bash
# Build for current platform
make build          # outputs dist/tui-streamer

# Run tests
make test           # go test ./...

# Lint
make lint           # go vet ./...

# macOS-specific
make build-darwin           # universal binary (arm64 + amd64 via lipo)
make app                    # .app with native WKWebView window (CGO required)
make app-server             # .app in headless server mode (opens browser, no CGO)
make dmg                    # distributable .dmg (requires 'make app' first)
make app BUNDLE=path.yaml   # .app named after BundleSet/Bundle metadata

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    Port to listen on (default: 8080)
  -dir string     Default working directory for executed commands (default: .)
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup
```

The `-open` flag defaults to `true` when the binary is running inside a macOS `.app` bundle (detected by checking whether the executable path contains `.app/Contents/MacOS/`). If the server cannot bind the port (another instance is already running), a macOS `.app` launch will open the browser to the existing instance and exit cleanly.

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method.
2. Write the handler as a method on `*Server`.
3. Access `s.manager` for session operations.
4. Respond with JSON using `json.NewEncoder(w).Encode(...)`.

### Adding a New Session Operation

1. Add method to `Session` in `internal/session/session.go`.
2. If it mutates shared state, protect with `s.mu` (RWMutex).
3. Wire it through a new REST endpoint in `server.go` if external access is needed.

---

## Key Conventions

### Go Style

- **Error handling**: wrap errors with `fmt.Errorf("context: %w", err)`.
- **Concurrency**: use `sync.RWMutex` for shared maps/state; `sync.Once` for one-time teardown; `context.Context` for cancellation.
- **Defers**: use `defer` for cleanup (mutex unlock, channel close, process teardown).
- **No global state** in internal packages — all state is injected via structs.
- Section separators in longer files use `// ──────` style comment lines.
- Pointer receivers on all non-trivial structs.

### JavaScript Style

- Vanilla JS, no framework, no build step — keep it that way.
- Classes for stateful components (`App`, `Terminal`, `SessionSocket`, `AnsiParser`, `MarkdownRenderer`).
- Always HTML-escape user/command output before inserting into the DOM.
- Themes are applied as a `data-theme` attribute on `<html>` (via `document.documentElement.setAttribute`). The default dark theme has no attribute set (or an empty string); other themes use their lowercase name (e.g. `data-theme="dracula"`).

### Adding a New Theme

1. Add a `[data-theme="<name>"]` block in `web/static/style.css` defining all CSS custom properties (see existing themes for the full variable list, including all `--ansi-*` colours).
2. Add the option to the `<select id="theme-select">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and applies it via `document.documentElement.setAttribute('data-theme', value)`.

### WebSocket Protocol

Messages are JSON objects sent as text frames:
```json
{"type":"start","timestamp":1700000000000}
{"type":"stdout","timestamp":1700000000123,"data":"hello world"}
{"type":"stderr","timestamp":1700000000456,"data":"error text"}
{"type":"exit","timestamp":1700000001000,"exit_code":0}
{"type":"error","timestamp":1700000001000,"data":"signal: killed"}
```

- `timestamp` is Unix milliseconds (`int64`), not an ISO string.
- `exit_code` is present only on `exit` lines; it is a pointer (`*int`) so zero exits are distinguishable from absent values.
- `data` is omitted on `start` and `exit` lines when empty.

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

### Bundle YAML Format

Bundle files use a Kubernetes-style manifest structure. A single file may contain multiple `---`-separated YAML documents.

**Single Bundle:**
```yaml
apiVersion: v1
kind: Bundle
metadata:
  name: Deploy
spec:
  sessions:
    - name: Build
      command: make build test
      description: Compile the project and run tests.
      autorun: true
    - name: Deploy
      command: make deploy
      autorun: false
```

**BundleSet (multiple bundles in one file):**
```yaml
apiVersion: v1
kind: BundleSet
metadata:
  name: My Runbook
spec:
  bundles:
    - name: Stage One
    - name: Stage Two
---
apiVersion: v1
kind: Bundle
metadata:
  name: Stage One
spec:
  sessions:
    - name: Prepare
      command: ./prepare.sh
      autorun: true
---
apiVersion: v1
kind: Bundle
metadata:
  name: Stage Two
spec:
  sessions:
    - name: Deploy
      command: ./deploy.sh
```

The `BundleSet` document controls display order and groups bundles under a shared name. If no `BundleSet` is present, all `Bundle` documents are included in document order. The top-level `File.Name` (used as the app title) is taken from the `BundleSet` name when present, or from the first `Bundle` name otherwise.

Session fields:
| Field | Type | Description |
|---|---|---|
| `name` | string | Display name shown in the sidebar |
| `command` | string | Shell command string (split on whitespace at runtime) |
| `description` | string | Markdown text rendered above terminal output |
| `autorun` | bool | Execute `command` automatically when the bundle loads |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `gopkg.in/yaml.v3` | YAML bundle file parsing |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## Security Notes

- **Command whitelisting**: use `-allow` flag in production to restrict which commands can be run. The whitelist checks the first token of the command (the binary name). An empty `AllowedCommands` slice permits everything.
- **No authentication**: the server assumes a trusted local network. Do not expose it publicly without adding auth.
- **WebSocket origin check** is permissive (`CheckOrigin` returns `true`) — appropriate for local dev, not for multi-tenant deployments.
- **HTML escaping**: `AnsiParser` and `MarkdownRenderer` in `app.js` escape all output before DOM insertion; do not bypass this.
- **Bundle body size**: the `/api/bundles` endpoint rejects request bodies over 4 MiB (`maxBundleBodyBytes`).

---

## macOS Packaging

The `scripts/package-macos.sh` script:
1. Creates `.app` bundle structure under `dist/`.
2. Injects the version string (from `git describe`) into `Info.plist`.
3. Optionally embeds a bundle YAML into `Resources/bundle.yaml` (read by `cmd/app/main.go` at launch).
4. Optionally code-signs with a provided identity or ad-hoc (`-`).
5. Optionally creates a `.dmg` with `hdiutil`.

Use `make app` for a native WKWebView window app, `make app-server` for a headless server app that opens the system browser.

When a bundle YAML is packaged (`make app BUNDLE=path.yaml`), the app's title and `.app` name are derived from the bundle's `BundleSet` or `Bundle` `metadata.name`.

The `cmd/app` entry point (WKWebView mode) uses a random free port to avoid conflicts, shows a loading splash while the HTTP server starts, and injects macOS keyboard shortcut handling (`⌘A/C/X/V/Z/⇧Z`) via `wv.Init()`.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay across page reloads
- Authentication / access control
- Windows packaging scripts
- Pre-built binaries for releases via the CI release workflow
