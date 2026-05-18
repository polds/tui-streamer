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
│   ├── bundle/bundle.go     # YAML bundle parser (Bundle / BundleSet kinds)
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
├── scripts/
│   └── package-macos.sh     # macOS .app/.dmg packaging script
├── go.mod                   # Go module (github.com/polds/tui-streamer)
├── go.sum                   # Dependency checksums
├── Makefile                 # Build, test, lint, packaging targets
└── README.md                # Minimal project description
```

---

## Architecture

### Backend (Go)

The backend uses a **session-based multiplexing** model:

1. **Session** (`internal/session/session.go`) — Named execution context. Holds state (ID, name, timestamps, running flag), a map of subscribed WebSocket clients, and a cancel function for the running process.
2. **Manager** (`internal/session/manager.go`) — Thread-safe registry (UUID → `*Session`). Provides Create/Get/List/Delete.
3. **Executor** (`internal/executor/executor.go`) — Spawns a process, reads stdout/stderr concurrently in separate goroutines, and emits `Line` structs (JSON) with timestamps and line type (`stdout`, `stderr`, `start`, `exit`, `error`).
4. **Client** (`internal/session/client.go`) — Wraps a `gorilla/websocket` connection with read/write pumps, a 256-element buffered send channel, ping/pong keepalive (54s), and `sync.Once`-guarded cleanup.
5. **Bundle** (`internal/bundle/bundle.go`) — Parses YAML bundle files containing `Bundle` and `BundleSet` documents. `Bundle.Load` reads a file; `Bundle.Parse` accepts raw bytes (also used by the `/api/bundles` endpoint).
6. **Server** (`internal/server/server.go`) — HTTP mux with:
 - `GET /` — serves embedded static files
 - `GET /ws/{id}` — upgrades to WebSocket, creates a Client, registers it to the session
 - `GET /api/sessions` — list all sessions
 - `POST /api/sessions` — create session
 - `GET /api/sessions/{id}` — get a single session
 - `DELETE /api/sessions/{id}` — delete session (also kills any running process)
 - `POST /api/sessions/{id}/exec` — execute command in session
 - `POST /api/sessions/{id}/kill` — kill running process
 - `POST /api/bundles` — parse a YAML bundle body and create its sessions

### Frontend (Vanilla JS)

`web/static/app.js` is a single-file application with no build step:

- **`AnsiParser`** — Converts ANSI SGR escape sequences to safe HTML spans (supports bold, dim, italic, underline, standard/256/true-color fg & bg).
- **`api`** — Thin wrapper over `fetch()` for all REST endpoints.
- **`SessionSocket`** — WebSocket wrapper with 500ms auto-reconnect on close.
- **`Terminal`** — Renders output lines with auto-scroll.
- **`App`** — Main controller: session creation/deletion, command dispatch, theme persistence (localStorage), per-session output buffering for replay.

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
make build              # outputs dist/tui-streamer

# Run tests
make test               # go test ./...

# Lint
make lint               # go vet ./...

# macOS-specific
make build-darwin       # universal binary (arm64 + amd64 via lipo)
make app                # .app bundle with native WKWebView window (requires CGO + Xcode)
make app-server         # .app bundle as headless server (opens browser; no CGO required)
make dmg                # distributable .dmg (requires 'make app' first)

# Clean
make clean
```

### Running the Server

```bash
./dist/tui-streamer [flags]

Flags:
  -port string    TCP port to listen on (default: "8080")
  -dir string     Default working directory for executed commands (default: ".")
  -title string   Window / browser-tab title (defaults to bundle name or "TUI Streamer")
  -stdout         Capture stdout (default true)
  -stderr         Capture stderr (default true)
  -allow string   Whitelist a binary name; repeat for multiple binaries
                  (e.g. -allow make -allow npm). Omit to allow all commands.
  -bundle string  Path to a YAML bundle file that pre-creates sessions
  -open           Auto-launch browser on startup
```

### Adding a New REST Endpoint

1. Add the route in `internal/server/server.go` inside the `routes()` method.
2. Write the handler as a method on `*Server`.
3. Access `s.manager` for session operations and `s.cfg` for server-level config.
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

Messages are newline-delimited JSON objects. `timestamp` is a **Unix epoch in milliseconds** (`int64`), not an ISO string.

```json
{"type":"start","timestamp":1704067200000}
{"type":"stdout","timestamp":1704067200123,"data":"hello"}
{"type":"stderr","timestamp":1704067200456,"data":"error text"}
{"type":"exit","timestamp":1704067200789,"exit_code":0}
```

- `data` is omitted (`omitempty`) on `start` and `exit` messages.
- `exit_code` is only present on `exit` messages; non-zero means failure.
- The `error` type is emitted when the process exits without an `ExitError` (e.g. killed by signal with no status).

Line types: `start`, `stdout`, `stderr`, `exit`, `error`.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/google/uuid` | Session ID generation |
| `github.com/gorilla/websocket` | WebSocket server implementation |
| `webview/webview_go` | macOS native WebView (CGO, darwin only, not in go.mod) |

Go standard library is used for HTTP, JSON, process execution, embedding, and synchronization — no web framework.

---

## Runtime Behaviour & Operational Notes

### Output buffering and replay
- The server retains up to **2 000** serialised JSON lines per session in `session.lineBuf`. Clients that connect after execution starts receive the buffered history immediately on subscribe.
- The frontend also caps both its in-memory replay buffer and visible DOM lines at **2 000** (`MAX_BUFFER` / `MAX_DOM_LINES` in `app.js`). Lines beyond this limit are silently dropped from the in-browser view.

### WebSocket backpressure
- Each `Client` has a **256-slot** non-blocking send channel. If the channel is full when the broadcaster tries to send, the message is **dropped silently** — there is no server-side persistence and no retry. Slow or stalled clients lose lines without any error signal.

### Process lifecycle
- When a session is killed (`POST /api/sessions/{id}/kill` or `DELETE /api/sessions/{id}`), stdout/stderr lines that arrive after cancellation are suppressed. The `exit` event is still forwarded so the UI can reflect the new state.
- After cancellation the executor enforces a **5-second drain window** (`waitDelay`) before forcibly closing pipes. This prevents goroutine leaks if the child process keeps writing after receiving a signal.

### Line-length limit
- `executor.go` uses `bufio.Scanner` with its default maximum token size (~64 KB). Lines longer than this without a newline will cause the scanner to return a `bufio.ErrTooLong` error; the goroutine exits and the session effectively ends without an explicit `exit` event.

### Command parsing
- `POST /api/sessions/{id}/exec` accepts `command` as either a **JSON string** (`"ls -la"`, split on whitespace) or a **JSON array** (`["ls", "-la"]`). There is no shell interpolation — quotes, glob patterns, and environment variable expansions in string form are not processed.
- The whitelist check (`-allow`) compares only the **first token** of the command array (the binary name).

### Bundle deduplication
- `POST /api/bundles` checks all existing sessions for a matching `bundle_name` before creating anything. If any bundle in the file has already been imported (by name), the entire request is rejected with `409 Conflict`. Re-importing requires deleting the existing sessions first.

### macOS .app behaviour
- `-open` defaults to `true` when the process detects it was launched from inside a `.app` bundle (executable path contains `.app/Contents/MacOS/`).
- If `ListenAndServe` fails inside a `.app` (port already in use), the server opens the browser to the existing instance URL and exits with code 0 — allowing double-click re-launch to work as a "bring to front" shortcut.

### Interactive commands
- The frontend maintains a hardcoded list (`INTERACTIVE_COMMANDS`) of well-known curses/TUI programs (`vim`, `htop`, `tmux`, etc.). Running one of these shows a confirmation prompt warning that the output will not render correctly without a real TTY.

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

| Target | Binary used | Window |
|---|---|---|
| `make app` | `cmd/app` (CGO, WKWebView) | Native WKWebView window |
| `make app-server` | `cmd/server` (no CGO) | Browser tab (opens automatically) |
| `make dmg` | Same as `make app` | Adds `.dmg` around the `.app` |

Pass `BUNDLE=path/to/bundle.yaml` to any of these targets to embed a bundle file and derive the app name from the bundle's `metadata.name`.

---

## What Does Not Exist Yet (Contribution Opportunities)

- Unit tests (no `*_test.go` files currently exist)
- CI/CD pipelines (no `.github/workflows/`)
- Session output persistence / history replay on page load (server-side; client-side replay buffer exists up to 2 000 lines)
- Authentication / access control
- Windows packaging scripts
- Pre-built release binaries
