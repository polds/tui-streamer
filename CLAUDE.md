# CLAUDE.md — AI Assistant Guide for tui-streamer

## Project Overview

**tui-streamer** is a Go-based WebSocket server that executes OS commands and streams their output (stdout/stderr) line-by-line to a web-based terminal UI in real time. It supports multiple concurrent named "sessions", each of which can have multiple WebSocket subscriber clients.

Key capabilities:
- Execute arbitrary CLI commands on demand via REST API
- Stream output line-by-line via WebSocket in JSON format
- Serve a self-contained, theme-able web UI (no build step required)
- Optional command whitelisting for security
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
│   ├── bundle/bundle.go     # YAML bundle parser (Bundle + BundleSet kinds)
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
│   ├── network-bundle/      # Multi-bundle network diagnostics example
│   └── lorem-ipsum/         # External API + streaming example
├── scripts/
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # Project description and usage guide
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Bundle** (`internal/bundle/bundle.go`) — YAML parser for bundle files. Supports two document kinds: `Bundle` (a named group of sessions) and `BundleSet` (an ordered list of `Bundle` references). A single file may contain multiple `---`-separated YAML documents. Exposes `Load(path)` and `Parse(data)`.
2. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, a cancel function for the running process, and a bounded replay buffer (up to 2,000 lines) so clients that connect after execution started receive prior output. Also carries optional bundle metadata: `PendingCommand`, `BundleName`, and `Description`.
3. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
4. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs with Unix-millisecond timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`). After the process exits or is cancelled, pipes are forcibly closed after a 5-second drain delay to prevent goroutine leaks.
5. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (ping every 54s, 60s pong timeout), and `sync.Once`-guarded cleanup. Messages are dropped (never block) when the buffer is full.
6. **Server** (`internal/server/server.go`) — HTTP mux with:
   - `GET /` — serves embedded static files (with server-side title injection)
   - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
   - `GET /api/sessions` — list all sessions
   - `POST /api/sessions` — create session
   - `GET /api/sessions/{id}` — get a single session
   - `DELETE /api/sessions/{id}` — delete session
   - `POST /api/sessions/{id}/exec` — execute command in session
   - `POST /api/sessions/{id}/kill` — kill running process
   - `POST /api/bundles` — import a YAML bundle (creates sessions, optionally autoruns commands)

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, blink, standard/256/true-color fg & bg).
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints including bundle import.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect on close.
- **`Terminal`** — Renders output lines with auto-scroll. Caps DOM to `MAX_DOM_LINES` (oldest stdout/stderr lines pruned first; event banners preserved).
- **`App`** — Main controller: session creation/deletion, command dispatch, kill, line selection/copy, bundle import, theme persistence (localStorage), per-session output buffering for replay.

### Data Flow

```
Browser (REST) → POST /api/sessions/{id}/exec
                       ↓
               server.go: validate, call session.Exec()
                       ↓
               executor.go: spawn process, read stdout/stderr
                       ↓
               session.go: broadcast Line JSON to all clients
                       ↓
               client.go: write to WebSocket send channel
                       ↓
               Browser (WebSocket) receives Line JSON → Terminal renders
```

---

## Development Workflows

### Prerequisites

- Go 1.22+
- `make`
- macOS with Xcode (only for the native app target)

### Common Commands

```bash
# Build for current platform
make build          # outputs dist/tui-streamer

# Run tests
make test           # go test ./...

# Lint
make lint           # go vet ./...

# macOS-specific
make build-darwin   # universal binary (arm64 + amd64 via lipo)
make app            # create .app bundle with native WKWebView window (requires CGO + Xcode)
make app-server     # create headless server .app (opens browser; no CGO needed)
make dmg            # create distributable .dmg (requires 'make app' first)
make icon           # generate AppIcon.icns from SVG (requires librsvg, macOS only)

# Bundle-specific .app (name derived from bundle metadata)
make app BUNDLE=./examples/network-bundle/bundle.yaml

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    TCP port to listen on (default: "8080")
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -dir string     Default working directory for executed commands (default: ".")
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple binaries
                  (omit to allow all commands)
  -bundle string  Path to a YAML bundle file that pre-creates sessions
                  with optional auto-execution
  -open           Auto-launch browser on startup (default true inside macOS .app)
```

**Notes on `-allow`**: the flag is repeated per binary, not comma-separated:

```bash
tui-streamer -allow make -allow npm -allow go
```

**Notes on `-bundle`**: the bundle's `BundleSet` or top-level `Bundle` `metadata.name` is used as the window title unless `-title` is also provided. Sessions with `autorun: true` start executing immediately on server startup.

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method called from `New()`.
2. Write the handler as a method on `*Server` or a closure.
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
- Classes for stateful components (`App`, `Terminal`, `SessionSocket`, `AnsiParser`).
- Always HTML-escape user/command output before inserting into the DOM.
- Theme names are CSS class names applied to `<body>`; add new themes in `style.css` using CSS custom properties.

### Adding a New Theme

1. Add a `body.theme-<name>` block in `web/static/style.css` defining all CSS variables (see existing themes for the full variable list).
2. Add the option to the `<select id="themeSelect">` in `web/static/index.html`.
3. No JS changes needed — the `App` class reads the selector value and applies it as a body class.

### WebSocket Protocol

Messages are JSON objects (one per WebSocket text frame):
```json
{"type":"start","timestamp":1704067200000}
{"type":"stdout","timestamp":1704067200100,"data":"output line"}
{"type":"stderr","timestamp":1704067200200,"data":"error text"}
{"type":"exit","timestamp":1704067200300,"exit_code":0}
{"type":"error","timestamp":1704067200400,"data":"signal: killed"}
```

Field notes:
- `timestamp` — Unix epoch in **milliseconds** (`int64`), not an ISO string.
- `data` — omitted on `start` and `exit` frames.
- `exit_code` — integer, present only on `exit` frames; `0` = success, non-zero = failure.

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

### Bundle System

Bundles allow pre-configuring sessions in a YAML file for startup (`-bundle` flag) or runtime import (UI or `POST /api/bundles`).

#### Document kinds

| Kind | Purpose |
|---|---|
| `Bundle` | Declares a named group of sessions |
| `BundleSet` | References multiple `Bundle` documents by name, controlling their order |

A single YAML file may contain multiple `---`-separated documents. A `BundleSet` and its referenced `Bundle` documents can all live in the same file.

#### `Bundle` schema

```yaml
apiVersion: v1
kind: Bundle
metadata:
  name: Deploy           # used as session group label and window title
spec:
  sessions:
    - name: Build        # display name shown in the sidebar
      description: |     # optional Markdown — rendered above terminal output
        Compile and test.
      command: make build test   # pre-populates the command bar
      autorun: true              # execute immediately on load
    - name: Deploy
      command: make deploy
      autorun: false
```

#### `BundleSet` schema

```yaml
apiVersion: v1
kind: BundleSet
metadata:
  name: Network Troubleshooting   # top-level name; becomes window title
spec:
  bundles:
    - name: Connectivity          # must match a Bundle metadata.name in the same file
    - name: DNS
```

#### Session fields populated from a bundle

| Field | JSON key | Description |
|---|---|---|
| `PendingCommand` | `pending_command` | Pre-populated in the command bar; executed if `autorun: true` |
| `BundleName` | `bundle_name` | Bundle the session belongs to; prevents duplicate imports |
| `Description` | `description` | Markdown string rendered in the terminal panel |

#### Duplicate import guard

`POST /api/bundles` checks existing sessions for a matching `bundle_name` before creating anything. If any bundle in the file has already been imported, the request is rejected with HTTP 409.

#### Adding a new bundle kind

1. Add a new case in the `switch d.Kind` block inside `internal/bundle/bundle.go:Parse()`.
2. Define a `*Spec` struct and decode `d.Spec` into it.
3. Append the resolved objects to `File`.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## Security Notes

- **Command whitelisting**: use `-allow` flag in production to restrict which commands can be run.
- **No authentication**: the server assumes a trusted local network. Do not expose it publicly without adding auth.
- **WebSocket origin check** is permissive (`CheckOrigin` returns `true`) — appropriate for local dev, not for multi-tenant deployments.
- **HTML escaping**: the `AnsiParser` in `app.js` escapes all output before DOM insertion; do not bypass this.

---

## macOS Packaging

The `scripts/package-macos.sh` script:
1. Creates `.app` bundle structure under `dist/`.
2. Injects the version string (from git tags) into `Info.plist`.
3. Optionally code-signs with a provided identity or ad-hoc (`-`).
4. Optionally creates a `.dmg` with `hdiutil`.

**Makefile targets:**

| Target | Binary | Description |
|---|---|---|
| `make app` | `cmd/app` (CGO) | WKWebView windowed app — opens a native macOS window; requires Xcode |
| `make app-server` | `cmd/server` (no CGO) | Headless server — opens the system browser; cross-compilable |
| `make dmg` | — | Wraps the `.app` from `make app` in a `.dmg` |

**Bundle packaging:** pass `BUNDLE=<path>` to name the `.app` after the bundle's metadata name:

```bash
make app BUNDLE=./examples/network-bundle/bundle.yaml
# → dist/Network Troubleshooting.app
```

**macOS `.app` behaviour quirk:** if the server port is already in use when launched as a `.app`, the process opens the browser to the existing instance and exits cleanly rather than reporting an error — this is handled by `insideAppBundle()` in `cmd/server/main.go`.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- Session output persistence / history replay on page load (server-side; the replay buffer is capped at 2,000 lines and lost on process restart)
- Authentication / access control
- Windows packaging scripts
